package main

import (
	"./blynk"
	//"encoding/hex"
	//"fmt"
	"math/rand"
)

func main() {
	temp := 25

	/*
		var m blynk.Message

		m.Build(blynk.CMD_LOGIN).PushString("0fee55cf5dc54ffd843c9478a5421226")

		fmt.Println(m)

		e, _ := m.MarshalBinary()

		fmt.Printf("binary:\n%s", hex.Dump(e))

		m.UnmarshalBinary(e)

		fmt.Println(m)
	*/

	client := blynk.NewClient(blynk.BLYNK_ADDRESS, "0fee55cf5dc54ffd843c9478a5421226")

	client.RegisterDeviceReaderFunction(2, func(pin uint, body *blynk.Body) {
		temp = temp + rand.Intn(3) - 1
		body.PushInt(temp)
	})

	client.Run()
}
