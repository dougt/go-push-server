package main

import (
	"./uuid"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"text/template"
)

const (
	HOST_NAME            = "localhost"
	PORT_NUMBER          = "8080"
	APPSERVER_API_PREFIX = "/notify/"
)

type Client struct {
	Websocket *websocket.Conn `json:"-"`
	UAID      string          `json:"uaid"`
	Ip        string          `json:"ip"`
	Port      float64         `json:"port"`
}

type Channel struct {
	UAID      string `json:"uaid"`
	ChannelID string `json:"channelID"`
	Version   string `json:"version"`
}

type ServerState struct {
	// Mapping from a UAID to the Client object
	ConnectedClients map[string]*Client `json:"connectedClients"`

	// Mapping from a UAID to all channels owned by that UAID
	UAIDToChannels map[string][]*Channel `json:"uaidToChannels"`

	// Mapping from a ChannelID to the cooresponding Channel
	ChannelIDToChannel map[string]*Channel `json:"channelIDToChannel"`
}

var gServerState ServerState

func openState() bool {

	var data []byte
	var err error

	data, err = ioutil.ReadFile("serverstate.json")
	if err != nil {
		log.Println("Could not read in serverstate")
		return false
	}

	err = json.Unmarshal(data, &gServerState)
	if err != nil {
		log.Println("Could not unmarshal serverstate")
		return false
	}

	return true
}

func saveState() {
	log.Println(" -> saving state..")

	var data []byte
	var err error

	data, err = json.Marshal(gServerState)
	if err != nil {
		return
	}
	ioutil.WriteFile("serverstate.json", data, 0644)
}

func handleRegister(client *Client, f map[string]interface{}) {
	log.Println(" -> handleRegister")

	if f["channelID"] == nil {
		log.Println("channelID is missing!")
		return
	}

	var channelID = f["channelID"].(string)

	// TODO https!
	var pushEndpoint = "http://" + HOST_NAME + ":" + PORT_NUMBER + APPSERVER_API_PREFIX + channelID

	channel := &Channel{client.UAID, channelID, ""}

	gServerState.UAIDToChannels[client.UAID] = append(gServerState.UAIDToChannels[client.UAID], channel)
	gServerState.ChannelIDToChannel[channelID] = channel

	type RegisterResponse struct {
		Name         string `json:"messageType"`
		Status       int    `json:"status"`
		PushEndpoint string `json:"pushEndpoint"`
		ChannelID    string `json:"channelID"`
	}

	register := RegisterResponse{"register", 200, pushEndpoint, channelID}

	j, err := json.Marshal(register)
	if err != nil {
		log.Println("Could not convert register response to json %s", err)
		return
	}

	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		// we could not send the message to a peer
		log.Println("Could not send message to ", client.Websocket, err.Error())
	}
}

func handleUnregister(client *Client, f map[string]interface{}) {
	log.Println(" -> handleUnregister")

	if f["channelID"] == nil {
		log.Println("channelID is missing!")
		return
	}

	var channelID = f["channelID"].(string)

	log.Println("len ", len(gServerState.ChannelIDToChannel))
	channel, ok := gServerState.ChannelIDToChannel[channelID]
	if ok {
		// only delete if UA owns this channel
		var index = -1
		for p, v := range gServerState.UAIDToChannels[client.UAID] {
			if v == channel {
				delete(gServerState.ChannelIDToChannel, channelID)
				index = p
			}
		}
		if index >= 0 {
			gServerState.UAIDToChannels[client.UAID] = append(gServerState.UAIDToChannels[client.UAID][:index], gServerState.UAIDToChannels[client.UAID][index+1:]...)
		}
	}
	log.Println("New len ", len(gServerState.ChannelIDToChannel))

	type UnregisterResponse struct {
		Name      string `json:"messageType"`
		Status    int    `json:"status"`
		ChannelID string `json:"channelID"`
	}

	unregister := UnregisterResponse{"unregister", 200, channelID}

	j, err := json.Marshal(unregister)
	if err != nil {
		log.Println("Could not convert unregister response to json %s", err)
		return
	}

	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		// we could not send the message to a peer
		log.Println("Could not send message to ", client.Websocket, err.Error())
	}
}

func handleHello(client *Client, f map[string]interface{}) {
	log.Println(" -> handleHello")
	gServerState.ConnectedClients[client.UAID] = client

	status := 200

	if f["uaid"] == nil {
		uaid, err := uuid.GenUUID()
		if err != nil {
			status = 400
			log.Println("GenUUID error %s", err)
		}
		client.UAID = uaid
	} else {
		client.UAID = f["uaid"].(string)

		// BUG(nikhilm): Does not deal with sending
		// a new UAID if their is a channel that was sent
		// by the UA which the server does not know about.
		// Which means in the case of this memory only server
		// it should actually always send a new UAID when it was
		// restarted
		if f["channelIDs"] != nil {
			for _, foo := range f["channelIDs"].([]interface{}) {
				channelID := foo.(string)
				log.Println("Got CHID ", channelID)
				c := &Channel{client.UAID, channelID, ""}
				gServerState.UAIDToChannels[client.UAID] = append(gServerState.UAIDToChannels[client.UAID], c)
				gServerState.ChannelIDToChannel[channelID] = c
			}
		}
	}

	if f["interface"] != nil {
		m := f["interface"].(map[string]interface{})
		client.Ip = m["ip"].(string)
		client.Port = m["port"].(float64)
	}

	type HelloResponse struct {
		Name   string `json:"messageType"`
		Status int    `json:"status"`
		UAID   string `json:"uaid"`
	}

	hello := HelloResponse{"hello", status, client.UAID}

	j, err := json.Marshal(hello)
	if err != nil {
		log.Println("Could not convert hello response to json %s", err)
		return
	}

	log.Println("going to send:  \n  ", string(j))
	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		log.Println("Could not send message to ", client.Websocket, err.Error())
	}
}

func handleAck(client *Client, f map[string]interface{}) {
	log.Println(" -> ack")
}

func pushHandler(ws *websocket.Conn) {

	log.Println("pushHandler: new websocket connection")

	client := &Client{ws, "", "", 0}

	for {
		var f map[string]interface{}

		var err error
		if err = websocket.JSON.Receive(ws, &f); err != nil {
			log.Println("Websocket Disconnected.", err.Error())
			break
		}

		log.Println("hi!")

		switch f["messageType"] {
		case "hello":
			handleHello(client, f)
			break

		case "register":
			handleRegister(client, f)
			break

		case "unregister":
			handleUnregister(client, f)
			break

		case "ack":
			handleAck(client, f)
			break

		default:
			log.Println(" -> Unknown", f)
			break
		}

		saveState()
	}
	log.Println("Closing Websocket!")
	ws.Close()
	gServerState.ConnectedClients[client.UAID].Websocket = nil
}

func notifyHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Got notification from app server ", r.URL)

	if r.Method != "PUT" {
		log.Println("NOT A PUT")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Method must be PUT."))
		return
	}

	channelID := strings.Replace(r.URL.Path, APPSERVER_API_PREFIX, "", 1)
	log.Println("channelID ", channelID)

	if strings.Contains(channelID, "/") {
		log.Println("Could not find a valid channelID")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Could not find a valid channelID."))
		return
	}

	value := r.FormValue("version")

	if value == "" {
		log.Println("Could not find version")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Could not find version."))
		return
	}

	channel, found := gServerState.ChannelIDToChannel[channelID]
	if !found {
		log.Println("Could not find channel " + channelID)
		return
	}
	channel.Version = value

	client := gServerState.ConnectedClients[channel.UAID]

	saveState()

	if client == nil {
		log.Println("no known client for the channel.")
	} else if client.Websocket == nil {
		wakeupClient(client)
	} else {
		sendNotificationToClient(client, channel)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func wakeupClient(client *Client) {

	// TODO probably want to do this a few times before
	// giving up.

	log.Println("wakeupClient: ", client)
	service := fmt.Sprintf("%s:%g", client.Ip, client.Port)

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

func sendNotificationToClient(client *Client, channel *Channel) {

	type NotificationResponse struct {
		Name     string    `json:"messageType"`
		Channels []Channel `json:"updates"`
	}

	var channels []Channel
	channels = append(channels, *channel)

	notification := NotificationResponse{"notification", channels}

	j, err := json.Marshal(notification)
	if err != nil {
		log.Println("Could not convert hello response to json %s", err)
		return
	}

	log.Println("going to send:  \n  ", string(j))
	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		log.Println("Could not send message to ", channel, err.Error())
	}
	log.Println("Sent notification to client")

}

func admin(w http.ResponseWriter, r *http.Request) {

	type User struct {
		UAID      string
		Connected bool
		Channels  []*Channel
	}

	type Arguments struct {
		PushEndpointPrefix string
		Users              []User
	}

	// TODO https!
	arguments := Arguments{"http://" + HOST_NAME + ":" + PORT_NUMBER + APPSERVER_API_PREFIX, nil}

	for k := range gServerState.UAIDToChannels {
		connected := gServerState.ConnectedClients[k] != nil
		u := User{k, connected, gServerState.UAIDToChannels[k]}
		arguments.Users = append(arguments.Users, u)
	}

	t := template.New("users.template")
	s1, _ := t.ParseFiles("templates/users.template")
	s1.Execute(w, arguments)
}

func main() {

	s := openState()

	if s == false {
		log.Println("Could not restore state")

		gServerState.UAIDToChannels = make(map[string][]*Channel)
		gServerState.ChannelIDToChannel = make(map[string]*Channel)
		gServerState.ConnectedClients = make(map[string]*Client)
	}

	http.HandleFunc("/admin", admin)

	http.Handle("/", websocket.Handler(pushHandler))

	http.HandleFunc(APPSERVER_API_PREFIX, notifyHandler)

	log.Println("Listening on", HOST_NAME+":"+PORT_NUMBER)
	log.Fatal(http.ListenAndServe(HOST_NAME+":"+PORT_NUMBER, nil))
}
