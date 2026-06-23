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
- Пользовательский хук очистки `OnDestroy(func(context.Context) error)`.
- Ограничение времени очистки через `SetTimeout(d)` (по таймауту колбэк
  получает отменённый контекст, возвращается `ErrShutdownTimeout`).
- Интеграция с `context.Context` через `WaitContext(ctx, ...)`.
- Опциональный логгер через интерфейс `Logger`.
- Ручная инициация остановки методом `End()` (неблокирующий, идемпотентный).
- Готовый к использованию глобальный экземпляр и пакетные алиасы
  (`Wait`, `WaitWithLogger`, `OnDestroy`, `End`), а также собственный
  экземпляр через `New()`.

## Установка

```bash
go get -u github.com/efureev/go-shutdown
```

## Примеры использования

Простейший вариант — дождаться сигнала завершения:

```go
import "github.com/efureev/go-shutdown"

func main() {
    // ... запуск приложения ...

    shutdown.Wait()
}
```

Ожидание конкретных сигналов с логгером:

```go
import (
    "syscall"

    "github.com/efureev/go-shutdown"
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

    "github.com/efureev/go-shutdown"
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

Отдельный экземпляр (рекомендуется вместо общего глобального состояния):

```go
sh := shutdown.New().
    SetTimeout(10 * time.Second).
    OnDestroy(func(ctx context.Context) error { return srv.Shutdown(ctx) })

if err := sh.Wait(); err != nil {
    log.Fatal(err)
}
```

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
