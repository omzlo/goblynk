package main

import (
	"fmt"
	"github.com/omzlo/goblynk"
	//"math/rand"
	"os"
)

func main() {
	//temp := 25

	key := os.Getenv("BLYNK_AUTH_TOKEN")
	if key == "" {
		fmt.Println("Missing BLYNK_AUTH_TOKEN environement variable")
		os.Exit(-2)
	}

	client := blynk.NewClient(blynk.BLYNK_ADDRESS, key)

	/*
		client.RegisterDeviceReaderFunction(2, func(pin uint, body *blynk.Body) {
			temp = temp + rand.Intn(3) - 1
			body.PushInt(temp)
		})
	*/

	client.OnConnect(func(c uint) error {
		client.Notify("Hello")
		return nil
	})

	client.Run()
}
