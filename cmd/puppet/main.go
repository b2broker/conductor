package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("Application started")
	exitSignal := make(chan os.Signal, 1)
	signal.Notify(exitSignal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	log.Println("Waiting for exit signal")
	<-exitSignal
	log.Println("Exited")
}
