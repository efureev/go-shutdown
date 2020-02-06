# Shutdown package for your app

## Install
```bash
go get -u github.com/efureev/go-shutdown
```

Golang app shutdown.

## Examples
```go
import "github.com/efureev/go-shutdown"

func main() {
	//..
    
    shutdown.Wait()
}
```

```go
import "github.com/efureev/go-shutdown"

func main() {
	//..
    
    shutdown.WaitWithLogger(logger, syscall.SIGINT, syscall.SIGTERM)
}
```
```go
import "github.com/efureev/go-shutdown"

func main() {
	//..
    
    shutdown.
        OnDestroy(func() {
            module.processing.EndJobListen()
        }).
        SetLogger(module.Log()).
        Wait()
}
```
