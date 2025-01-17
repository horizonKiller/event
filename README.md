<p align="center">
<img 
    src="logo.png" 
    width="350" height="175" border="0" alt="event">
<br><br>
<a title="Build Status" target="_blank" href="https://github.com/cheng-zhongliang/event/actions?query=workflow"><img src="https://img.shields.io/github/actions/workflow/status/cheng-zhongliang/event/go.yml?logo=github-actions" /></a>
<a title="Codecov" target="_blank" href="https://codecov.io/gh/cheng-zhongliang/event"><img src="https://img.shields.io/codecov/c/github/cheng-zhongliang/event?logo=codecov" /></a>
<a title="Go Report Card" target="_blank" href="https://goreportcard.com/report/github.com/cheng-zhongliang/event"><img src="https://goreportcard.com/badge/github.com/cheng-zhongliang/event" /></a>
<a title="Doc for event" target="_blank" href="https://pkg.go.dev/github.com/cheng-zhongliang/event"><img src="https://img.shields.io/badge/go.dev-doc-007d9c?logo=read-the-docs" /></a>
<a href="https://github.com/cheng-zhongliang/event/blob/master/LICENSE"><img src="https://img.shields.io/badge/license-BSD--3--Clause-brightgreen"></a>
<a title="Release" target="_blank" href="https://github.com/cheng-zhongliang/event/releases"><img src="https://img.shields.io/github/v/release/cheng-zhongliang/event.svg?color=161823&logo=smartthings" /></a>
<a title="Tag" target="_blank" href="https://github.com/cheng-zhongliang/event/tags"><img src="https://img.shields.io/github/v/tag/cheng-zhongliang/event?color=%23ff8936&logo=fitbit" /></a>
<a title="Require Go Version" target="_blank" href="https://github.com/cheng-zhongliang/event"><img src="https://img.shields.io/badge/go-%3E%3D1.20-30dff3?logo=go" /></a>
<a title="Supported Platforms" target="_blank" href="https://github.com/cheng-zhongliang/event"><img src="https://img.shields.io/badge/platform-Linux-549688?logo=launchpad" /></a>
</p>

`event` is a network I/O event notification library for Go. It uses [epoll](https://en.wikipedia.org/wiki/Epoll) to poll I/O events that is fast and low memory usage. It works in a similar manner as [libevent](https://github.com/libevent/libevent).

The goal of `event` is to provide a `BASIC` tool for building high performance network applications.

*Note: All development is done on a Raspberry Pi 4B.*

## Features

- Supports more events
- Flexible timer and ticker
- Supports event priority
- Edge-triggered
- Simple API
- Low memory usage

## Getting Started

### Installing
To start using `event`, just run `go get`:

```sh
$ go get -u github.com/cheng-zhongliang/event
```

### Events

- `EvRead` fires when the fd is readable.
- `EvWrite` fires when the fd is writable.
- `EvClosed` fires when the connection has closed.
- `EvTimeout` fires when the timeout expires.
- `EvSignal` fires when the os signal arrives.
- `EvPersist` __if not set, the event will be deleted after it is triggered.__

When the event is triggered, the callback function will be called.

### Read/Write/Closed/Timeout

These events can be used in combination.

```go
base := event.NewBase()
ev := event.New(fd, event.EvRead|event.Timeout|event.EvPersist, callback, arg)
base.AddEvent(ev, 1*time.Second)
```

When the fd is readable or timeout expires, this event will be triggered.

### Signal

The signal event will be triggered when the os signal arrives.

```go
base := event.NewBase()
ev := event.NewSignal(os.Interrupt, callback, arg)
base.AddEvent(ev, 0)
```

### Timer

The timer is a one-shot event that will be triggered after the timeout expires.

```go
base := event.NewBase()
ev := event.NewTimer(callback, arg)
base.AddEvent(ev, 1*time.Second)
```

### Ticker

The ticker is a repeating event that will be triggered every time the timeout expires.

```go
base := event.NewBase()
ev := event.NewTicker(callback, arg)
base.AddEvent(ev, 1*time.Second)
```

### Priority

When events are triggered together, high priority events will be dispatched first.

```go
ev := event.New(fd, event.EvRead|event.EvET, callback, arg)
ev.SetPriority(event.High)
```

### Edge-triggered

The event is level-triggered by default. If you want to use edge-triggered, you can set the `EvET` flag.

```go
ev := event.New(fd, event.EvRead|event.EvET, callback, arg)
```

### Usage

Example echo server that binds to port 1246:

```go
package main

import (
	"syscall"

	"github.com/cheng-zhongliang/event"
)

func main() {
	base, err := event.NewBase()
	if err != nil {
		panic(err)
	}

	fd := Socket()
	ev := event.New(fd, event.EvRead|event.EvPersist, Accept, base)
	if err := base.AddEvent(ev, 0); err != nil {
		panic(err)
	}

	exitEv := event.NewSignal(syscall.SIGINT, Exit, base)
	if err := base.AddEvent(exitEv, 0); err != nil {
		panic(err)
	}

	if err := base.Dispatch(); err != nil && err != event.ErrBadFileDescriptor {
		panic(err)
	}
}

func Socket() int {
	addr := syscall.SockaddrInet4{Port: 1246, Addr: [4]byte{0, 0, 0, 0}}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, syscall.IPPROTO_TCP)
	if err != nil {
		panic(err)
	}
	if err := syscall.Bind(fd, &addr); err != nil {
		panic(err)
	}
	err = syscall.Listen(fd, syscall.SOMAXCONN)
	if err != nil {
		panic(err)
	}
	return fd
}

func Accept(fd int, events uint32, arg interface{}) {
	base := arg.(*event.EventBase)

	clientFd, _, err := syscall.Accept(fd)
	if err != nil {
		panic(err)
	}

	ev := event.New(clientFd, event.EvRead|event.EvPersist, Echo, nil)
	if err := base.AddEvent(ev, 0); err != nil {
		panic(err)
	}
}

func Echo(fd int, events uint32, arg interface{}) {
	buf := make([]byte, 0xFFF)
	n, err := syscall.Read(fd, buf)
	if err != nil {
		panic(err)
	}
	if _, err := syscall.Write(fd, buf[:n]); err != nil {
		panic(err)
	}
}

func Exit(fd int, events uint32, arg interface{}) {
	base := arg.(*event.EventBase)

	if err := base.Exit(); err != nil {
		panic(err)
	}
}
```

Connect to the echo server:

```sh
$ telnet localhost 1246
```
