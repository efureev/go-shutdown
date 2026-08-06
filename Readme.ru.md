[![Test](https://github.com/efureev/go-shutdown/actions/workflows/test.yml/badge.svg)](https://github.com/efureev/go-shutdown/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/go-shutdown/v3.svg)](https://pkg.go.dev/github.com/efureev/go-shutdown/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/go-shutdown/v3)](https://goreportcard.com/report/github.com/efureev/go-shutdown/v3)

# Shutdown

> Read this in other languages: [English](Readme.md)

`go-shutdown` — небольшой пакет без зависимостей для **graceful shutdown**
Go-приложений и сервисов.

Он ожидает сигналы операционной системы (по умолчанию `SIGINT`, `SIGTERM`,
`SIGQUIT`), отмену контекста либо ручную команду, а затем выполняет ваши хуки
очистки — закрытие соединений, остановку воркеров, сброс буферов — перед
завершением процесса.

## Возможности

- **Произвольное число именованных хуков очистки.** Выполняются в порядке,
  обратном регистрации (LIFO), — подсистемы гасятся в порядке, обратном запуску.
- **Параллельные группы.** Подряд идущие хуки с `Parallel()` выполняются
  одновременно; обычный хук служит барьером между группами.
- **Таймауты на двух уровнях.** `WithTimeout` ограничивает всю
  последовательность, `HookTimeout` — отдельный хук. По таймауту ошибка
  называет хуки, которые не успели.
- **Каждый хук выполняется, даже если предыдущий вернул ошибку.** Ошибки
  объединяются и оборачиваются в `HookError`, так что `errors.As` доходит до
  конкретного сбоя.
- **Наблюдаемость без блокировки.** `Done()` и `Context()` позволяют воркерам
  реагировать на остановку, не вызывая `Wait`.
- **Принудительное завершение.** Сигнал, пришедший во время очистки, завершает
  процесс с кодом `128+signum` — зависший хук можно прервать.
- **Определение причины.** `Reason()`, `Signal()`, `ExitCode()`.
- **Структурное логирование** через `*slog.Logger`. По умолчанию молчит.
- **Ноль зависимостей.** Только стандартная библиотека.

## Установка

```bash
go get -u github.com/efureev/go-shutdown/v3
```

## Использование

Простейший случай — дождаться сигнала завершения, без очистки:

```go
import (
    "context"

    "github.com/efureev/go-shutdown/v3"
)

func main() {
    // ... запуск приложения ...

    if err := shutdown.Wait(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

Несколько подсистем, останавливаемых в порядке, обратном запуску:

```go
sh := shutdown.New(
    shutdown.WithTimeout(15*time.Second),
    shutdown.WithLogger(slog.Default()),
)

sh.Add("db", func(context.Context) error { return db.Close() })
sh.Add("cache", func(ctx context.Context) error { return cache.Flush(ctx) }, shutdown.Parallel())
sh.Add("search", func(ctx context.Context) error { return search.Flush(ctx) }, shutdown.Parallel())
sh.Add("http", func(ctx context.Context) error { return srv.Shutdown(ctx) })

// порядок остановки: http, затем cache и search вместе, затем db
if err := sh.Wait(context.Background()); err != nil {
    log.Printf("очистка завершилась с ошибкой: %v", err)
}

os.Exit(sh.ExitCode())
```

Воркеры реагируют на остановку, не блокируясь в `Wait`:

```go
for {
    select {
    case <-sh.Done():
        return
    case job := <-jobs:
        process(job)
    }
}
```

`sh.Context()` — тот же сигнал в виде `context.Context`: его можно передавать
дальше и выводить из него дочерние. Его `context.Cause` — `ErrShutdown`.

Узнать, какой именно хук упал:

```go
var hookErr *shutdown.HookError
if errors.As(sh.Wait(ctx), &hookErr) {
    log.Printf("подсистема %q не остановилась: %v", hookErr.Hook, hookErr.Err)
}
```

Отдельный бюджет для медленной подсистемы:

```go
sh.Add("drain", drainQueue, shutdown.HookTimeout(5*time.Second))
```

По истечении `Wait` возвращает ошибку, оборачивающую `ErrTimeout` (а значит и
`context.DeadlineExceeded`), с именами хуков, которые не успели. Хук,
игнорирующий свой контекст, продолжает работать в своей горутине, поэтому
длительная очистка обязана уважать `ctx`, чтобы быть прерываемой.

## Ручная остановка

`End()` запускает остановку из кода. Неблокирующий, идемпотентный, может
вызываться до или после `Wait`.

Экземпляр одноразовый: после завершения очистки `Wait` немедленно возвращает
тот же результат, и все конкурентные вызовы `Wait` получают его же.

## Переход с v2

API изменился существенно — см. [MIGRATION.md](MIGRATION.md).

## Лицензия

См. файл [LICENSE](LICENSE).
