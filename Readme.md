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
        OnDestroy(func(done chan<- bool) {
            module.processing.EndJobListen()
            done <- true
        }).
        SetLogger(module.Log()).
        Wait()
}
```
