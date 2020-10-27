package main

import (
    "flag"
    "http_capture"
)

func main() {
    var port int
    flag.IntVar(&port, "port", 8888, "-port port")
    flag.Parse()
    http_capture.Run(port)
}
