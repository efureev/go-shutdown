[![Test](https://github.com/efureev/go-shutdown/actions/workflows/test.yml/badge.svg)](https://github.com/efureev/go-shutdown/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/go-shutdown.svg)](https://pkg.go.dev/github.com/efureev/go-shutdown)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/go-shutdown)](https://goreportcard.com/report/github.com/efureev/go-shutdown)

# Shutdown

`go-shutdown` — небольшой пакет для **graceful shutdown** Go-приложений и
сервисов.

Он блокирует выполнение и ожидает сигналы операционной системы
(по умолчанию `SIGINT`, `SIGTERM`, `SIGQUIT`), а при их получении выполняет
вашу функцию очистки (закрытие соединений, остановка воркеров, сброс буферов
и т.п.) перед завершением процесса.

## Возможности

- Ожидание стандартных или произвольных сигналов ОС.
- Произвольное число хуков очистки: `Add(name, func(context.Context) error)`
  и безымянный `OnDestroy(func(context.Context) error)`. Хуки выполняются в
  порядке, обратном регистрации (LIFO); каждый выполняется, даже если
  предыдущий вернул ошибку, а ошибки объединяются и возвращаются.
- Ограничение времени очистки через `SetTimeout(d)` — единый бюджет на всю
  последовательность хуков (по таймауту хуки получают отменённый контекст,
  возвращается `ErrShutdownTimeout`).
- Интеграция с `context.Context` через `WaitContext(ctx, ...)`. Контекст,
  передаваемый хукам, отвязан от него: отмена контекста, который вы ожидаете,
  запускает очистку, а не прерывает её.
- Принудительное завершение: сигнал, пришедший во время очистки, завершает
  процесс с кодом `128+signum` — зависший хук можно прервать. Отключается
  через `SetForceOnSecondSignal(false)`.
- Определение причины остановки: `Reason()` (`ReasonSignal`, `ReasonContext`,
  `ReasonManual`), `Signal()` и `ExitCode()` по конвенции `128+signum`.
- Опциональный логгер через интерфейс `Logger`.
- Ручная инициация остановки методом `End()` (неблокирующий, идемпотентный).
- Готовый к использованию глобальный экземпляр и пакетные алиасы
  (`Wait`, `WaitWithLogger`, `OnDestroy`, `Add`, `End`), а также собственный
  экземпляр через `New()`.

`Shutdown` выполняет свои хуки один раз. Повторный `Wait` на экземпляре, чья
очистка уже завершилась, немедленно возвращает ту же ошибку, а конкурентные
вызовы `Wait` получают результат одного и того же прогона очистки.

## Переход с v2.0.x

**`OnDestroy` теперь добавляет** хук, а не заменяет ранее зарегистрированный.
Раньше второй вызов молча терял первый колбэк — очистка пропадала всякий раз,
когда два компонента использовали общий `DefaultShutdown`. Если вы полагались
на поведение с заменой, вызовите сначала `ResetHooks()`.

**Сигнал, полученный во время очистки, теперь завершает процесс.** Раньше он
проглатывался, и зависший хук можно было прервать только через `SIGKILL`.
Если очистку нельзя прерывать, вызовите `SetForceOnSecondSignal(false)`.

**Повторный `Wait` больше не блокируется навсегда** — он возвращает ошибку уже
выполненной очистки. Это делает экземпляр одноразовым: для нескольких циклов
остановки (например, в тестах) нужен новый экземпляр из `New()`.

## Установка

```bash
go get -u github.com/efureev/go-shutdown/v2
```

## Примеры использования

Простейший вариант — дождаться сигнала завершения:

```go
import "github.com/efureev/go-shutdown/v2"

func main() {
    // ... запуск приложения ...

    shutdown.Wait()
}
```

Ожидание конкретных сигналов с логгером:

```go
import (
    "syscall"

    "github.com/efureev/go-shutdown/v2"
)

func main() {
    // ... запуск приложения ...

    shutdown.WaitWithLogger(logger, syscall.SIGINT, syscall.SIGTERM)
}
```

С функцией очистки и логгером (колбэк получает `context.Context` и
возвращает `error`):

```go
import (
    "context"

    "github.com/efureev/go-shutdown/v2"
)

func main() {
    // ... запуск приложения ...

    err := shutdown.
        OnDestroy(func(ctx context.Context) error {
            return module.processing.EndJobListen(ctx)
        }).
        SetLogger(module.Log()).
        Wait()
    if err != nil {
        // обработка ошибки очистки
    }
}
```

Несколько подсистем, останавливаемых в порядке, обратном запуску:

```go
sh := shutdown.New().SetTimeout(15 * time.Second)

sh.Add("http", func(ctx context.Context) error { return srv.Shutdown(ctx) })
sh.Add("consumer", func(ctx context.Context) error { return consumer.Stop(ctx) })
sh.Add("db", func(context.Context) error { return db.Close() })

// При остановке: db → consumer → http. Сбой одного хука не останавливает
// остальные; err объединяет все ошибки, каждая помечена именем своего хука.
if err := sh.Wait(); err != nil {
    log.Printf("остановка завершилась с ошибками: %v", err)
}
```

Отдельный экземпляр (рекомендуется вместо общего глобального состояния):

```go
sh := shutdown.New().
    SetTimeout(10 * time.Second).
    OnDestroy(func(ctx context.Context) error { return srv.Shutdown(ctx) })

if err := sh.Wait(); err != nil {
    log.Fatal(err)
}
```

Сообщить, почему процесс остановился, и выйти с соответствующим кодом:

```go
sh := shutdown.New().OnDestroy(func(ctx context.Context) error { return srv.Shutdown(ctx) })

if err := sh.Wait(); err != nil {
    log.Printf("очистка завершилась с ошибкой: %v", err)
}

// reason=signal signal=terminated code=143
log.Printf("reason=%s signal=%v code=%d", sh.Reason(), sh.Signal(), sh.ExitCode())

os.Exit(sh.ExitCode())
```

`Signal()` возвращает `nil`, если `Reason()` не равен `ReasonSignal`, а
`ExitCode()` даёт `0` для любой причины, кроме сигнала. Ошибки очистки на код
выхода не влияют — это решение вызывающего.

Остановка по сигналу либо по отмене внешнего контекста:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := shutdown.New().WaitContext(ctx); err != nil {
    log.Fatal(err)
}
```

## Лицензия

См. файл [LICENSE](LICENSE).
