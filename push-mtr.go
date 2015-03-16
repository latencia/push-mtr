package main

import (
	"encoding/json"
	"fmt"
	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	log "github.com/Sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v1"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
)

var (
	mqttClient *mqtt.MqttClient
	MTR_BIN    = "/usr/bin/mtr"
)

func runUrlGet(scheme, host, topic string) {
	// if empty, do skip this test
	if scheme != "" {
		if err := urlGet(scheme+"://"+host, topic); err != nil {
			log.Error(err)
		}
	}
}

func runMtrReport(count int, host string, loc *ReportLocation, stdout bool, topic string) (err error) {
	r := NewReport(count, host, loc)

	if stdout {
		msg, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(msg))
	} else {
		msg, _ := json.Marshal(r)
		pushMsg(topic, string(msg))
	}

	return err
}

func main() {
	kingpin.Version(PKG_VERSION)

	count := kingpin.Flag("count", "Report cycles (mtr -c)").
		Default("10").Int()

	topic := kingpin.Flag("topic", "MTTQ topic").Default("/metrics/mtr").
		String()

	urlGetTopic := kingpin.Flag("url-get-topic", "MTTQ topic").Default("/metrics/url-get").
		String()

	host := kingpin.Arg("host", "Target host").Required().String()

	repeat := kingpin.Flag("repeat", "Send the report every X seconds").
		Default("0").Int()

	brokerUrls := kingpin.Flag("broker-urls", "Comman separated MQTT broker URLs").
		Required().Default("").OverrideDefaultFromEnvar("MQTT_URLS").String()

	stdout := kingpin.Flag("stdout", "Print the report to stdout").
		Default("false").Bool()

	cafile := kingpin.Flag("cafile", "CA certificate when using TLS (optional)").
		String()

	location := kingpin.Flag("location", "Geocode the location of the server").
		String()

	insecure := kingpin.Flag("insecure", "Don't verify the server's certificate chain and host name.").
		Default("false").Bool()

	debug := kingpin.Flag("debug", "Print debugging messages").
		Default("false").Bool()

	enablePprof := kingpin.Flag("enable-pprof", "Enable runtime profiling via pprof").
		Default("false").Bool()

	clientID := kingpin.Flag("clientid", "Use a custom MQTT client ID").String()

	furlGet := kingpin.Flag("url-get", "Report URL GET metrics").String()

	kingpin.Parse()

	log.Info("Starting push-mtr")

	var err error

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	if *enablePprof {
		go func() {
			fmt.Println(http.ListenAndServe("127.0.0.1:6161", nil))
		}()
	}

	if *clientID == "" {
		*clientID, err = os.Hostname()
		if err != nil {
			log.Fatal("Can't get the hostname to use it as the ClientID, use --clientid option")
		}
	}
	log.Debugf("MQTT Client ID: %s", *clientID)

	if *cafile != "" {
		if _, err := os.Stat(*cafile); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading CA certificate %s", err.Error())
			os.Exit(1)
		}
	}

	MTR_BIN = findMtrBin()
	if MTR_BIN == "" {
		fmt.Fprintf(os.Stderr, "mtr command not found in path\n")
		os.Exit(1)
	}

	loc, err := findLocation(*location)
	if err != nil {
		log.Fatalf("Geocoding of location %s failed: %s", *location, err)
		os.Exit(1)
	}

	urlList := parseBrokerUrls(*brokerUrls)
	tlsConfig := newTlsConfig(*cafile, *insecure)
	mqttClient, err = newMqttClient(urlList, clientID, tlsConfig)

	runTests := func() {
		go runUrlGet(*furlGet, *host, *urlGetTopic, loc)
		if err := runMtrReport(*count, *host, loc, *stdout, *topic); err != nil {
			log.Errorf("Error running mtr report: %s\n", err)
		}
	}

	if *repeat != 0 {
		timer := time.NewTicker(time.Duration(*repeat) * time.Second)
		for range timer.C {
			runTests()
		}
	} else {
		runTests()
	}
}
