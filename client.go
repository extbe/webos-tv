package webostv

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"log"
	"net"
	"net/url"
)

const (
	modelNameTag = "<modelName>LG TV</modelName>"
)

var (
	errNoDevicesDiscovered       = errors.New("no devices were discovered")
	errMultipleDevicesDiscovered = errors.New("multiple devices were discovered, please specify more concrete keyword")
	errUnsupportedRegResponse    = errors.New("unsupported registration response was received")
	errFailedToRegister          = errors.New("failed to register TV client")
	errUnsupportedWsRspType      = errors.New("unsupported websocket response type")

	//go:embed registration-payload.json
	registrationPayload string
)

type callback func(rsp wsResponse)

type Client interface {
	Connect() error
	Disconnect() error
	SendBlocking(msg Message) (wsResponse, error)
}

type ConfigStore interface {
	GetClientKey() (string, error)
	SetClientKey(key string) error
}

func New(config ConfigStore) (Client, error) {
	return NewWithKeyword(config, modelNameTag)
}

func NewWithKeyword(config ConfigStore, keyword string) (Client, error) {
	discoveredURLs, err := discover("urn:schemas-upnp-org:device:MediaRenderer:1", keyword)
	if err != nil {
		return nil, err
	}

	if len(discoveredURLs) == 0 {
		return nil, errNoDevicesDiscovered
	}

	if len(discoveredURLs) > 1 {
		return nil, errMultipleDevicesDiscovered
	}

	c := defaultClient{
		config:    config,
		deviceURL: discoveredURLs[0],
	}

	return &c, nil
}

type defaultClient struct {
	config    ConfigStore
	deviceURL string
	wsConn    *websocket.Conn
	writeChan chan []byte
	readChan  chan wsResponse
	done      chan struct{}
	callbacks map[string]callback
}

func (c *defaultClient) Connect() error {
	parsedURL, err := url.Parse(c.deviceURL)
	if err != nil {
		return err
	}

	host, _, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return err
	}

	wsURL := url.URL{
		Scheme: "ws",
		Host:   host + ":3000",
	}

	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		return err
	}

	message, err := createRegistrationMessage(c.config)
	if err != nil {
		return err
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	err = wsConn.WriteMessage(websocket.TextMessage, messageBytes)
	if err != nil {
		return err
	}

	proceed := true
	for proceed {
		_, rspMessage, err := wsConn.ReadMessage()
		if err != nil {
			return err
		}

		var rsp wsResponse
		err = json.Unmarshal(rspMessage, &rsp)
		if err != nil {
			return err
		}

		switch rsp.Type {
		case wsRspSuccessType:
			if "PROMPT" == rsp.Payload["pairingType"].(string) {
				log.Println("Please accept the connection on TV")
			} else {
				return fmt.Errorf("%w: %s", errUnsupportedRegResponse, string(rspMessage))
			}
		case wsRspErrorType:
			return fmt.Errorf("%w: %s", errFailedToRegister, rsp.Error)
		case wsRspRegisteredType:
			clientKey := rsp.Payload["client-key"].(string)
			err := c.config.SetClientKey(clientKey)
			if err != nil {
				return err
			}
			proceed = false
		default:
			return fmt.Errorf("%w: %s", errUnsupportedWsRspType, rsp.Type)
		}
	}

	c.wsConn = wsConn
	c.writeChan = make(chan []byte, 1)
	c.callbacks = make(map[string]callback)

	go c.writeLoop()
	go c.readLoop()

	return nil
}

func createRegistrationMessage(config ConfigStore) (map[string]interface{}, error) {
	var payload map[string]interface{}
	err := json.Unmarshal([]byte(registrationPayload), &payload)
	if err != nil {
		return nil, err
	}

	key, err := config.GetClientKey()
	if err != nil {
		return nil, err
	}

	if "" != key {
		payload["client-key"] = key
	}

	message := map[string]interface{}{
		"type":    "register",
		"id":      uuid.New().String(),
		"payload": payload,
	}

	return message, nil
}

func (c *defaultClient) writeLoop() {
	for {
		select {
		case msg := <-c.writeChan:
			err := c.wsConn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println("failed to send message: " + err.Error())
			}
		case <-c.done:
			return
		}
	}
}

func (c *defaultClient) readLoop() {
	go func() {
		forwardMessages(c.wsConn, c.readChan)
	}()

	for {
		select {
		case msg := <-c.readChan:
			cb, exists := c.callbacks[msg.ID]
			if exists {
				delete(c.callbacks, msg.ID)
				go cb(msg)
			}
		case <-c.done:
			return
		}
	}
}

func forwardMessages(wsConn *websocket.Conn, outChan chan wsResponse) {
	for {
		var msg wsResponse

		err := wsConn.ReadJSON(&msg)
		if err != nil {
			if errors.Is(err, &json.UnmarshalTypeError{}) {
				log.Println("failed to unmarshal message: " + err.Error())
				continue
			} else {
				// todo: forward is stopped atm, but the rest is working as usual.
				// how to stop ws or swallow error but the we need to react to websocket closing
				panic("failed to read WebSocket: " + err.Error())
			}
		}

		outChan <- msg
	}
}

func (c *defaultClient) Disconnect() error {
	err := c.wsConn.Close()
	if err != nil {
		return err
	}

	return nil
}

func (c *defaultClient) SendBlocking(msg Message) (wsResponse, error) {
	msgJson, err := json.Marshal(msg)
	if err != nil {
		return wsResponse{}, err
	}

	callbackChan := make(chan wsResponse, 1)
	c.callbacks[msg.ID] = func(rsp wsResponse) {
		callbackChan <- rsp
	}

	c.writeChan <- msgJson

	rsp := <-callbackChan
	close(callbackChan)

	if rsp.Type == wsRspErrorType {
		err = fmt.Errorf("%s", rsp.Error)
	}

	return rsp, err
}
