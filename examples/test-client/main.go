// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	var address string
	var mode string
	flag.StringVar(&address, "address", "0.0.0.0:8080", "server address")
	flag.StringVar(&mode, "mode", "tcp", "tcp or udp")
	flag.Parse()

	conn, err := net.Dial(mode, address)
	if err != nil {
		fmt.Printf("error initializing connection (%v)\n", err)
		os.Exit(1)
	}

	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Printf("error closing connection (%v)\n", err)
			os.Exit(1)
		}
	}(conn)

	fmt.Print("enter 'e' to exit\n")

	reader := bufio.NewReader(os.Stdin)
	res := make([]byte, 1000)
	for {
		fmt.Print("hex: ")
		raw, _ := reader.ReadString('\n')
		str := strings.Trim(raw, "\n ")

		if str == "e" {
			break
		}

		packet, err := hex.DecodeString(str)
		if err != nil {
			fmt.Printf("unable to decode hex input (%v)\n", err)
			continue
		}

		if _, err := conn.Write(packet); err != nil {
			fmt.Printf("send error (%v)\n", err)
			return
		}

		n, err := conn.Read(res)
		if err != nil {
			fmt.Printf("read error (%v)\n", err)
			return
		}

		fmt.Printf("server response: %s\n", hex.EncodeToString(res[:n]))
	}
}
