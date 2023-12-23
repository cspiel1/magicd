package main

import (
	"fmt"
	"regexp"
	"net"
	"os"
	"os/signal"
	"syscall"
	"strconv"
	"io/ioutil"
	"encoding/json"

	magichome "github.com/moonliightz/magic-home/pkg"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func checkError(e error) {
	if e != nil {
		fmt.Println("Error: ", e)
		os.Exit(1)
	}
}

type MHController struct {
	name   string
	ip     net.IP
	port   uint16

	c *magichome.Controller
}

func processMessage(c *magichome.Controller, msg mqtt.Message) error {
	var err error

	reg_on, _ := regexp.Compile("light/.*/on")
	reg_value, _ := regexp.Compile("light/.*/value")
	topic := msg.Topic()

	switch {
	case reg_on.MatchString(topic):
		if string(msg.Payload()) == "True" {
			err = c.SetState(magichome.On)
		} else {
			err = c.SetState(magichome.Off)
		}

	case reg_value.MatchString(topic):
		var v int
		v, err = strconv.Atoi(string(msg.Payload()))
		if err != nil { break }
		err = c.SetColor(magichome.Color{
			R: uint8(v),
			G: uint8(v),
			B: uint8(v),
			W: 0,
		})
	default:
	}

	return err
}

func mqttMessageHandler(mh *MHController) mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		var err error
		fmt.Printf("Received message: %s from topic: %s\n",
			   msg.Payload(), msg.Topic())

		err = processMessage(mh.c, msg)
		if err != nil {
			fmt.Println("Connection to magic-home lost. " + 
					"Reconnecting ...");
			c, err := magichome.New(mh.ip, mh.port)
			mh.c.Close()
			checkError(err)
			mh.c = c
			processMessage(mh.c, msg)
		}
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
    fmt.Println("Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
    fmt.Printf("Connect lost: %v\n", err)
}

func mainloop() {
    c := make(chan os.Signal)
    signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

    // block until a signal is received
    s := <-c
    fmt.Println("Got signal:", s)
}


type MHControllerConfig struct {
	Name string
	Ip string
	Port string
}


type Conf struct {
	Mqtt_host string
	Mqtt_port string
	Mqtt_user string
	Mqtt_pass string
	Controllers []MHControllerConfig
}


/**
* Creates a new Magic Home Controller
*
* @param client               MQTT client
* @param MHControllerConfig   Magic home controller config
*
* @return                     The created MHController
*/
func addController(client mqtt.Client, mhcfg MHControllerConfig) MHController {
	port, err := strconv.ParseUint(mhcfg.Port, 10, 64)
	mh := MHController {
		ip:    net.ParseIP(mhcfg.Ip),
		port:  uint16(port),
	}

	c, err := magichome.New(mh.ip, mh.port)
	checkError(err)

	mh.c = c
	topic := fmt.Sprintf("light/%s/#", mhcfg.Name)
	client.Subscribe(topic, 1, mqttMessageHandler(&mh))

	return mh
}


func main() {
	var rc    = ".magicdrc"

	file, err := ioutil.ReadFile(".magicdrc")
	if err != nil {
		fmt.Println("Could not read .magicdrc\n", err)
		os.Exit(1)
	}

	var cfg Conf
	err = json.Unmarshal(file, &cfg)
	if err != nil {
		fmt.Println("Wrong format .magicdrc\n", err)
		os.Exit(1)
	}

	fmt.Printf("broker: %s:%s\n", cfg.Mqtt_host, cfg.Mqtt_port);

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s",
			cfg.Mqtt_host, cfg.Mqtt_port))
	opts.SetUsername(cfg.Mqtt_user)
	opts.SetPassword(cfg.Mqtt_pass)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() {
		checkError(token.Error())
	}

	var mhctrls []MHController
	for _ , c := range cfg.Controllers {
		fmt.Printf("   magic home controller: %s %s:%s\n",
		c.Name, c.Ip, c.Port)
		mhctrls = append(mhctrls, addController(client, c))
	}

	fmt.Printf("BEGIN\n")
	mainloop()
	fmt.Printf("END\n")

	// And finaly close the connection to LED Strip Controller
	for _, mh := range mhctrls {
		mh.c.Close()
	}

	client.Disconnect(250)
}
