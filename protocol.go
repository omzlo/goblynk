package blynk

import (
	//"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type DeviceReader interface {
	DeviceRead(pin uint, param *Body)
}

type DeviceWriter interface {
	DeviceWrite(pin uint, param Body)
}

type DeviceWriterFunctionWrapper struct {
	deviceWrite func(pin uint, param Body)
}

func NewDeviceWriterFunctionWrapper(fn func(pin uint, param Body)) *DeviceWriterFunctionWrapper {
	return &DeviceWriterFunctionWrapper{fn}
}

func (w DeviceWriterFunctionWrapper) DeviceWrite(pin uint, param Body) {
	w.deviceWrite(pin, param)
}

type DeviceReaderFunctionWrapper struct {
	deviceRead func(pin uint, param *Body)
}

func NewDeviceReaderFunctionWrapper(fn func(pin uint, param *Body)) *DeviceReaderFunctionWrapper {
	return &DeviceReaderFunctionWrapper{fn}
}

func (r DeviceReaderFunctionWrapper) DeviceRead(pin uint, param *Body) {
	r.deviceRead(pin, param)
}

type Client struct {
	mutex        sync.Mutex
	conn         *net.TCPConn
	msgIdOut     uint16
	msgIdIn      uint16
	lastActivity time.Time
	address      string
	authKey      string
	readers      map[uint]DeviceReader
	writers      map[uint]DeviceWriter
	onConnect    func(uint) error
	state        int
}

const BLYNK_ADDRESS = "blynk-cloud.com:8442"

const (
	STATE_ERROR = iota
	STATE_CONNECTING
	STATE_CONNECTED
)

const (
	CMD_RESPONSE                = 0
	CMD_REGISTER                = 1
	CMD_LOGIN                   = 2
	CMD_SAVE_PROF               = 3
	CMD_LOAD_PROF               = 4
	CMD_GET_TOKEN               = 5
	CMD_PING                    = 6
	CMD_ACTIVATE                = 7
	CMD_DEACTIVATE              = 8
	CMD_REFRESH                 = 9
	CMD_GET_GRAPH_DATA          = 10
	CMD_GET_GRAPH_DATA_RESPONSE = 11

	CMD_TWEET         = 12
	CMD_EMAIL         = 13
	CMD_NOTIFY        = 14
	CMD_BRIDGE        = 15
	CMD_HARDWARE_SYNC = 16
	CMD_INTERNAL      = 17
	CMD_SMS           = 18
	CMD_PROPERTY      = 19
	CMD_HARDWARE      = 20
)

const (
	STATUS_SUCCESS                  = 200
	STATUS_QUOTA_LIMIT_EXCEPTION    = 1
	STATUS_ILLEGAL_COMMAND          = 2
	STATUS_NOT_REGISTERED           = 3
	STATUS_ALREADY_REGISTERED       = 4
	STATUS_NOT_AUTHENTICATED        = 5
	STATUS_NOT_ALLOWED              = 6
	STATUS_DEVICE_NOT_IN_NETWORK    = 7
	STATUS_NO_ACTIVE_DASHBOARD      = 8
	STATUS_INVALID_TOKEN            = 9
	STATUS_ILLEGAL_COMMAND_BODY     = 11
	STATUS_GET_GRAPH_DATA_EXCEPTION = 12
	STATUS_NO_DATA_EXCEPTION        = 17
	STATUS_DEVICE_WENT_OFFLINE      = 18
	STATUS_SERVER_EXCEPTION         = 19
	STATUS_NTF_INVALID_BODY         = 13
	STATUS_NTF_NOT_AUTHORIZED       = 14
	STATUS_NTF_ECXEPTION            = 15
	STATUS_TIMEOUT                  = 16
	STATUS_NOT_SUPPORTED_VERSION    = 20
	STATUS_ENERGY_LIMIT             = 21
)

/* */

func NewClient(addr, auth string) *Client {
	return &Client{address: addr, authKey: auth, readers: make(map[uint]DeviceReader), writers: make(map[uint]DeviceWriter)}
}

func (client *Client) Run() {
	log.Printf("Starting goblynk\n")
	client.runCycle()
}

func (client *Client) VirtualWrite(pin uint, v ...interface{}) error {
	var response Message

	response.Build(CMD_HARDWARE).PushString("vw").PushInt(int(pin))

	for _, item := range v {
		switch t := item.(type) {
		case byte:
			response.Body.PushByte(t)
		case int:
			response.Body.PushInt(t)
		case string:
			response.Body.PushString(t)
		case fmt.Stringer:
			response.Body.PushString(t.String())
		default:
			fmt.Printf("VirtualWrite does not know how to process %T\n", t)
			panic("Type error")
		}
	}
	return client.sendMessage(response)
}

func (client *Client) Notify(notification string) error {
	var msg Message

	msg.Build(CMD_NOTIFY).PushString(notification)

	return client.sendMessage(msg)
}

func (client *Client) RegisterDeviceReader(pin uint, r DeviceReader) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	client.readers[pin] = r
}

func (client *Client) RegisterDeviceReaderFunction(pin uint, fn func(pin uint, body *Body)) {
	client.RegisterDeviceReader(pin, NewDeviceReaderFunctionWrapper(fn))
}

func (client *Client) UnregisterDeviceReader(pin uint) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	delete(client.readers, pin)
}

func (client *Client) RegisterDeviceWriter(pin uint, w DeviceWriter) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	client.writers[pin] = w
}

func (client *Client) RegisterDeviceWriterFunction(pin uint, fn func(pin uint, body Body)) {
	client.RegisterDeviceWriter(pin, NewDeviceWriterFunctionWrapper(fn))
}

func (client *Client) UnregisterDeviceWriter(pin uint) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	delete(client.writers, pin)
}

func (client *Client) OnConnect(fn func(uint) error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()

	client.onConnect = fn
}

/* private */

func (client *Client) runCycle() {
	var msg Message
	var connectionCount uint = 0

	go client.heartbeatRunCycle()
	for {

		backoff := 3 * time.Second
		for {
			if err := client.connect(); err != nil {
				log.Printf("Connection to %s failed: %s", client.address, err)
				log.Printf("Waiting %s before attempting to reconnect.", backoff)
				time.Sleep(backoff)
				if backoff < 192*time.Second {
					backoff = 2 * backoff
				}
			} else {
				break
			}
		}

		connectionCount++
		log.Printf("Connected and authenticated to %s, connection cycle %d\n", client.address, connectionCount)

		if client.onConnect != nil {
			if err := client.onConnect(connectionCount); err != nil {
				log.Printf("OnConnect returned an error: %s", err)
				client.conn.Close()
				return
			}
		}

		for client.state == STATE_CONNECTED {
			if err := client.recvMessage(&msg); err != nil {
				break
			}

			log.Printf("Recieved message: %s", msg)

			switch msg.Header.Command {
			case CMD_RESPONSE:
				if msg.Header.Length != STATUS_SUCCESS {
					log.Printf("Recieved error status %d", msg.Header.Length)
					client.state = STATE_ERROR
				}
			case CMD_HARDWARE:
				client.processHardwareCommand(msg)
			case CMD_PING:
				msg.Build(CMD_RESPONSE)
				msg.Header.Length = STATUS_SUCCESS
				client.sendMessage(msg)
			}
		}

		log.Printf("Closing connection...")
		client.conn.Close()
	}
}

func (client *Client) processHardwareCommand(m Message) {
	var response Message

	fn, ok := m.Body.AsString(0)

	if !ok {
		log.Printf("Missing function in hardware message body %s, ignoring", m.Body)
		return
	}

	switch fn {
	case "vw":
		pin, ok := m.Body.AsInt(1)
		if !ok {
			log.Printf("Missing pin in hardware message, ignoring")
			return
		}
		if writer, ok := client.writers[uint(pin)]; ok {
			m.Body.Shift(2)
			log.Printf("Calling virtual pin writer %d with parameters %s", pin, m.Body)
			writer.DeviceWrite(uint(pin), m.Body)
		} else {
			log.Printf("Ignoring write to virtual pin %d: no handler", pin)
		}
	case "vr":
		pin, ok := m.Body.AsInt(1)
		if !ok {
			log.Printf("Missing pin in hardware message, ignoring")
			return
		}
		if reader, ok := client.readers[uint(pin)]; ok {
			response.Build(CMD_HARDWARE).PushString("vw").PushInt(int(pin))
			//response.Header.Id = m.Header.Id
			log.Printf("Calling virtual pin reader %d", pin)
			reader.DeviceRead(uint(pin), &response.Body)
			client.sendMessage(response)
		} else {
			log.Printf("Ignoring read to virtual pin %d: no handler", pin)
		}
	default:
		log.Printf("Ignoring hardware command '%s'", fn)
	}
}

func (client *Client) heartbeatRunCycle() {
	var ping Message
	for {
		if client.state == STATE_CONNECTED && time.Since(client.lastActivity).Seconds() >= 10.0 {
			ping.Build(CMD_PING)
			client.sendMessage(ping)
		}
		time.Sleep(1 * time.Second)
	}
}

func (client *Client) connect() error {
	var msg Message
	var response Message
	var err error

	addr, err := net.ResolveTCPAddr("tcp", client.address)
	if err != nil {
		return err
	}

	client.msgIdOut = 0
	client.msgIdIn = 0

	client.state = STATE_CONNECTING

	if client.conn, err = net.DialTCP("tcp", nil, addr); err != nil {
		return err
	}
	client.conn.SetNoDelay(true)

	log.Printf("Connected to %s", client.address)

	msg.Build(CMD_LOGIN).PushString(client.authKey)

	if err = client.sendMessage(msg); err != nil {
		return err
	}

	if err = client.recvMessage(&response); err != nil {
		return err
	}

	if response.Header.Length != 200 {
		return fmt.Errorf("Connection refused: reason code=%d", response.Header.Length)
	}
	client.state = STATE_CONNECTED
	return nil
}

func (client *Client) sendMessage(m Message) error {
	var err error

	client.mutex.Lock()
	defer client.mutex.Unlock()

	if m.Header.Id == 0 {
		client.msgIdOut++
		m.Header.Id = client.msgIdOut
	}

	if m.Header.Command == CMD_RESPONSE {
		data, _ := m.Header.MarshalBinary()
		log.Printf("Sending header %s", m.Header)
		_, err = client.conn.Write(data)

	} else {
		data, _ := m.MarshalBinary()
		log.Printf("Sending message %s", m)
		_, err = client.conn.Write(data)
	}

	if err != nil {
		return err
	}
	client.lastActivity = time.Now()
	return nil
}

func (client *Client) recvMessage(m *Message) error {
	var buf [2000]byte

	log.Printf("Waiting data to read")
	if _, err := client.conn.Read(buf[0:5]); err != nil {
		return err
	}
	m.Body.Clear()
	m.Header.UnmarshalBinary(buf[0:5])

	if m.Header.Command != CMD_RESPONSE {
		if m.Header.Length > uint16(len(buf)) {
			return fmt.Errorf("Message body too big: %d bytes requested", m.Header.Length)
		}
		if _, err := client.conn.Read(buf[5 : m.Header.Length+5]); err != nil {
			return err
		}
		m.Body.UnmarshalBinary(buf[5 : m.Header.Length+5])
	}
	log.Printf("Received %s", m)
	/*
		if m.Header.Command != CMD_RESPONSE {
			fmt.Print(hex.Dump(buf[:m.Header.Length+5]))
		} else {
			fmt.Print(hex.Dump(buf[:5]))
		}
	*/
	return nil
}
