![Go package](https://github.com/efureev/go-shutdown/workflows/Go%20package/badge.svg?branch=master)
[![Build Status](https://travis-ci.com/efureev/go-shutdown.svg?branch=master)](https://travis-ci.com/efureev/go-shutdown)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/go-shutdown)](https://goreportcard.com/report/github.com/efureev/go-shutdown)
[![Codacy Badge](https://api.codacy.com/project/badge/Grade/b9b3b425d3b34069a4094ef99a982a85)](https://www.codacy.com/manual/efureev/go-shutdown?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=efureev/go-shutdown&amp;utm_campaign=Badge_Grade)
[![Maintainability](https://api.codeclimate.com/v1/badges/b5c1678bafd0687f3070/maintainability)](https://codeclimate.com/github/efureev/go-shutdown/maintainability)


# Shutdown 
It's a package for graceful shutdown your app or process

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
