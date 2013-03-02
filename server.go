package main

import (
        "code.google.com/p/go.net/websocket"
        "net/http"
        "log"
	"encoding/json"
	"strings"
	"./uuid"
)

const (
	HOST_NAME = "localhost"
	PORT_NUMBER = "8080"
	APPSERVER_API_PREFIX = "/notify/"
)

type Client struct {
	Websocket *websocket.Conn
	UAID string
}

type Channel struct {
	ChannelID string `json:"channelID"`
	Version string   `json:"version"`
}

var uaid_to_channel map[string][]Channel;
var channel_to_client map[string] *Client;

func handleRegister(client *Client, f map[string]interface{}) {

	log.Println(" -> handleRegister");

	if (f["channelID"] == nil) {
		log.Println("channelID is missing!");
		return;
	}

	var channelID = f["channelID"].(string);
	var pushEndpoint = "http://" + HOST_NAME + ":" + PORT_NUMBER + APPSERVER_API_PREFIX + channelID;

	uaid_to_channel[client.UAID] = append(uaid_to_channel[client.UAID], Channel{channelID, ""});
	channel_to_client[channelID] = client;

	type RegisterResponse struct {
		Name string          `json:"messageType"`
		Status int           `json:"status"`
		PushEndpoint string  `json:"pushEndpoint"`
		ChannelID string      `json:"channelID"`
	}

	register := RegisterResponse{"register", 200, pushEndpoint, channelID}

	j, err := json.Marshal(register);
	if err != nil || string(j) == "null" {
		log.Println("Could not convert hello response to json %s",err)
		return;
        }

	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		// we could not send the message to a peer
		log.Println("Could not send message to ", client.Websocket, err.Error())
	}
}

func handleHello(client *Client, f map[string]interface{}) {
	log.Println(" -> handleHello");

	status := 200;

	if (f["uaid"] == nil) {
		uaid, err := uuid.GenUUID()
		if  err != nil {
			status = 400;
			log.Println("GenUUID error %s",err)
		}
		client.UAID = uaid;
	} else {
		client.UAID = f["uaid"].(string)
	}

	type HelloResponse struct {
		Name string          `json:"messageType"`
		Status int           `json:"status"`
		UAID string          `json:"uaid"`
		Channels []Channel   `json:"channelIDs"`
	}

	hello := HelloResponse{"hello", status, client.UAID, uaid_to_channel[client.UAID]}

	j, err := json.Marshal(hello);
	if err != nil || string(j) == "null" {
		log.Println("Could not convert hello response to json %s",err)
		return;
        }

	// update the channel_to_client table
//	for channel := range uaid_to_channel[client.UAID] {
//		channel_to_client[channel.ChannelID] = client;
//	}
	 

	log.Println("going to send:  \n  ", string(j));
	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		log.Println("Could not send message to ", client.Websocket, err.Error())
	}
}

func handleAck(client *Client, f map[string]interface{}) {
	log.Println(" -> ack");
}

func pushHandler(ws *websocket.Conn) {

	client := Client{ws, ""}

	for {
		var f map[string]interface{}

		var err error
		if err = websocket.JSON.Receive(ws, &f); err != nil {
			log.Println("Websocket Disconnected.", err.Error())
			ws.Close();
			return;
		}

		switch f["messageType"] {
		case "hello":
			handleHello(&client, f);
			break;
		case "register":
			handleRegister(&client, f);
			break;

		case "ack":
			handleAck(&client, f);
			break;
		default:
			log.Println(" -> Unknown", f);
			break;
		}
	}
}

func notifyHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != "PUT" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Method must be PUT."))
		return
	}

	channelID := strings.Trim(r.URL.Path, APPSERVER_API_PREFIX)

	if (strings.Contains(channelID, "/") || len(channelID) != (32+4)) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Could not find a valid channelID."))
		return;
	}
	

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Could not find a valid version."))
		return;
	}

	values := r.Form["version"]

	if (len(values) != 1) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Could not find one version."))
		return;
	}

	version := values[0];
	client := channel_to_client[channelID];
	if (client == nil) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Channel ID."))
		return;
	}
	
	uaid := client.UAID;
	log.Println("uaid: ", uaid, "channelID: ", channelID, " version: ", version);

	// update the channel with the new version.
	for i := range uaid_to_channel[uaid] {
		if (uaid_to_channel[uaid][i].ChannelID == channelID) {
			uaid_to_channel[uaid][i].Version = version;
		}
	}

	type NotificationResponse struct {
		Name string          `json:"messageType"`
		Channels []Channel   `json:"updates"`
	}

	notification := NotificationResponse{"notification",  uaid_to_channel[client.UAID]}

	j, err := json.Marshal(notification);
	if err != nil || string(j) == "null" {
		log.Println("Could not convert hello response to json %s",err)
		return;
        }

	log.Println("going to send:  \n  ", string(j));
	if err = websocket.Message.Send(client.Websocket, string(j)); err != nil {
		log.Println("Could not send message to ", client.Websocket, err.Error())
	}
	
}

func main() {

	uaid_to_channel = make(map[string][]Channel)
	channel_to_client = make(map[string] *Client);

	http.Handle("/", http.FileServer(http.Dir(".")))

	http.Handle("/push", websocket.Handler(pushHandler))
	http.HandleFunc(APPSERVER_API_PREFIX, notifyHandler);

	log.Println("Listening on ", HOST_NAME + ":" + PORT_NUMBER );
	log.Fatal(http.ListenAndServe(HOST_NAME  + ":" + PORT_NUMBER , nil))
}

