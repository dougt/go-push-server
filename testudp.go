package main

import (
        "fmt"
	"net"
	"os"
	"log"
)

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error ", err.Error())
		os.Exit(1)
	}
}

func WakeupClient(service string) {

	udpAddr, err := net.ResolveUDPAddr("up4", service)
	if err != nil {
		log.Println("ResolveUDPAddr error ", err.Error())
		return
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Println("DialUDP error ", err.Error())
		return
	}

	_, err = conn.Write([]byte("anything"))
	if err != nil {
		log.Println("UDP Write error ", err.Error())
		return
	}

}

func main() {

	log.Println("Testing UDP!");

	service := "localhost:2442"
	WakeupClient(service);
}
