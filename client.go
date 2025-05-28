package main

import (
	"encoding/json"
	"fmt"
	"net"
)

type LoginPayload struct {
	Username string
	Password string
}

func StartClient() {
	conn, err := net.Dial("tcp", "localhost:9000")
	if err != nil {
		fmt.Println("Kết nối thất bại:", err)
		return
	}
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var msg string
	decoder.Decode(&msg)
	fmt.Println(msg)

	fmt.Print("Tên đăng nhập: ")
	var username string
	fmt.Scanln(&username)
	fmt.Print("Mật khẩu: ")
	var password string
	fmt.Scanln(&password)

	encoder.Encode(LoginPayload{Username: username, Password: password})

	var response string
	decoder.Decode(&response)
	fmt.Println(response)

	for {
		decoder.Decode(&response)
		fmt.Println(response)
		var input string
		fmt.Scanln(&input)
		encoder.Encode(input)
		decoder.Decode(&response)
		fmt.Println(response)
	}
}
