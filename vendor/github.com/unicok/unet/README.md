# unet
net skeleton for go

[![Build Status](https://travis-ci.org/unicok/unet.svg?branch=master)](https://travis-ci.org/unicok/unet)
[![codecov](https://codecov.io/gh/unicok/unet/branch/master/graph/badge.svg)](https://codecov.io/gh/unicok/unet)
[![Go Report Card](https://goreportcard.com/badge/github.com/unicok/unet)](https://goreportcard.com/report/github.com/unicok/unet)

exmaple
==========

```go
package main

import (
    "log"

    "github.com/unicok/unet"
    "github.com/unicok/unet/codec"
)

type AddReq struct {
    A, B int
}

type AddRsp struct {
    C int
}

func main() {
    json := codec.Json()
    json.Register(AddReq{})
    json.Register(AddRsp{})

    server, err := unet.Serve("tcp", "0.0.0.0:0", json, 0 /* sync send */)
    checkErr(err)
    addr := server.Listener().Addr().String()
    go server.Serve(unet.HandlerFunc(serverSessionLoop))

    client, err := unet.Connect("tcp", addr, json, 0)
    checkErr(err)
    clientSessionLoop(client)
}

func serverSessionLoop(session *unet.Session) {
    for {
        req, err := session.Receive()
        checkErr(err)

        err = session.Send(&AddRsp{
            req.(*AddReq).A + req.(*AddReq).B,
        })
        checkErr(err)
    }
}

func clientSessionLoop(session *unet.Session) {
    for i := 0; i < 10; i++ {
        err := session.Send(&AddReq{
            i, i,
        })
        checkErr(err)
        log.Printf("Send: %d + %d", i, i)

        rsp, err := session.Receive()
        checkErr(err)
        log.Printf("Receive: %d", rsp.(*AddRsp).C)
    }
}

func checkErr(err error) {
    if err != nil {
        log.Fatal(err)
    }
}
```