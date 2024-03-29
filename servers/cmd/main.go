package main

import (
	"flag"
	"fmt"
	"github.com/alexandrevicenzi/go-sse"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/jw3/ppc/servers"
	"github.com/xujiajun/gorouter"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func mqttOpts(cfg *servers.ServerConfig) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.BrokerURI)
	opts.SetClientID(cfg.ClientID)
	opts.SetKeepAlive(2 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	return opts
}

func req2chan(r *http.Request) string {
	return strings.TrimLeft(r.URL.Path, "/v1/events/")
}

func main() {
	cfg := servers.NewServerConfiguration()

	mqttUri := flag.String("mqtt", cfg.BrokerURI, "uri of mqtt server")
	appPre := flag.String("app-prefix", cfg.AppPrefix, "event topic prefix")
	eventCh := flag.String("event-prefix", cfg.EventChannelId, "event topic prefix")
	functionCh := flag.String("function-prefix", cfg.FunctionChannelId, "function topic prefix")
	flag.Parse()

	if *mqttUri == "" || *eventCh == "" || *functionCh == "" {
		flag.Usage()
		return
	}

	cfg.BrokerURI = *mqttUri
	cfg.AppPrefix = *appPre
	cfg.EventChannelId = *eventCh
	cfg.FunctionChannelId = *functionCh

	log.Printf("mqtt @ %s", cfg.BrokerURI)

	eventTopic := fmt.Sprintf("%s/%s", cfg.AppPrefix, cfg.EventChannelId)
	functionTopic := fmt.Sprintf("%s/%s", cfg.AppPrefix, cfg.FunctionChannelId)

	c := mqtt.NewClient(mqttOpts(cfg))
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	// channel for shuttling mqtt events
	events := make(chan [2]string)

	if token := c.Subscribe(eventTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		t := strings.TrimLeft(msg.Topic(), eventTopic)
		events <- [2]string{t, string(msg.Payload())}
		println(t)
	}); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		os.Exit(1)
	}

	so := sse.Options{ChannelNameFunc: req2chan}
	s := sse.NewServer(&so)
	defer s.Shutdown()

	mux := gorouter.New()

	// Non-cloud functions
	http.HandleFunc("/health", handleHealth)

	//
	// Cloud functions
	//

	// GET /v1/events/{EVENT_NAME}
	mux.GET("/v1/events/{t:\\S+}", func(w http.ResponseWriter, r *http.Request) {
		s.ServeHTTP(w, r)
	})

	// POST /v1/devices/{DEVICE_ID}/{FUNCTION}
	mux.POST("/v1/devices/:device/:function", func(w http.ResponseWriter, r *http.Request) {
		d := gorouter.GetParam(r, "device")
		f := gorouter.GetParam(r, "function")
		t := fmt.Sprintf("%s/%s/%s", functionTopic, d, f)
		eb := r.FormValue("args")
		b, _ := url.QueryUnescape(eb)

		println(fmt.Sprintf("function called %s => %s", t, b))

		e := c.Publish(t, 0, false, b)

		if e.Error() == nil {
			w.WriteHeader(http.StatusOK)
		} else {
			println(e.Error().Error())
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	// GET /v1/devices/events/{EVENT_PREFIX}
	mux.POST("/v1/devices/events", func(w http.ResponseWriter, r *http.Request) {
		t := r.FormValue("name")
		d := r.FormValue("data")

		tt := fmt.Sprintf("%s/%s", cfg.EventChannelId, t)
		println(fmt.Sprintf("event published: %s data: %s", tt, d))

		c.Publish(tt, 0, false, d)
		events <- [2]string{t, d}

		w.WriteHeader(http.StatusOK)
	})
	http.Handle("/", mux)

	go func() {
		for {
			e := <-events
			t := e[0]
			s.SendMessage(t, sse.NewMessage("", e[1], t))
			//println(fmt.Sprintf("t: %s", t))
			for _, c := range s.Channels() {
				//println(fmt.Sprintf("tt: %s", tt))
				//println(fmt.Sprintf("%s ? %s", c, t))
				if c != t && strings.HasPrefix(t, c) {
					//println(fmt.Sprintf("======%s==%s======", c, t))
					s.SendMessage(c, sse.NewMessage("", e[1], t))
				}
			}
		}
	}()

	log.Fatal(http.ListenAndServe(":9000", nil))
}

func handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
}
