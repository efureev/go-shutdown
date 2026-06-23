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
- Пользовательский хук очистки `OnDestroy(func() error)`.
- Опциональный логгер через интерфейс `ILogger`.
- Ручная инициация остановки методом `End()`.
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

С функцией очистки и логгером (обратите внимание: колбэк возвращает `error`):

```go
import "github.com/efureev/go-shutdown"

func main() {
    // ... запуск приложения ...

    err := shutdown.
        OnDestroy(func() error {
            return module.processing.EndJobListen()
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
    OnDestroy(func() error { return srv.Close() })

if err := sh.Wait(); err != nil {
    log.Fatal(err)
}
```

## Документация и анализ

- `docs/REVIEW.md` — критический анализ пакета.
- `docs/TASKS.md` — ТЗ на исправление багов и недочётов.
- `docs/IMPROVEMENTS.md` — предложения по развитию.

## Лицензия

См. файл [LICENSE](LICENSE).
