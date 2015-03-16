package main

// MQTT boilerplate

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	log "github.com/Sirupsen/logrus"
	"io/ioutil"
	"net/url"
	"os"
	"strings"
	"time"
)

// Test SSL connections to the brokers because the current
// paho mqtt client implementation returns a generic error message
// hard to debug.
func isTLSOK(uri *url.URL, config *tls.Config) bool {
	_, err := tls.Dial("tcp", uri.Host, config)

	if err != nil {
		log.Warnf("Ignoring broker %s: %s", uri.String(), err)
		return false
	}

	return true
}

func pushMsg(topic, msg string) bool {
	_, open := <-mqttClient.Publish(mqtt.QOS_ONE, topic, msg)
	// paho closes the channel if there's an error sending
	if !open {
		return false
	}
	return true
}

// tcp://user:password@host:port
func newMqttClient(brokerUrls []string, clientID *string, tlsConfig *tls.Config) (*mqtt.MqttClient, error) {

	opts := mqtt.NewClientOptions()
	opts.SetCleanSession(true)
	opts.SetWriteTimeout(10 * time.Second)

	opts.SetClientId(*clientID)
	for _, broker := range brokerUrls {
		uri, err := url.Parse(broker)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing broker url (ignored): %s\n", broker)
			continue
		}
		if uri.Scheme == "ssl" {
			if !isTLSOK(uri, tlsConfig) {
				continue
			}
			opts.SetTlsConfig(tlsConfig)
		}
		opts.AddBroker(broker)
	}

	client := mqtt.NewClient(opts)
	_, err := client.Start()
	if err != nil {
		return &mqtt.MqttClient{}, fmt.Errorf("Connection to the broker(s) failed: %s", err)
	}
	return client, nil
}

func newTlsConfig(cacertPath string, verify bool) *tls.Config {
	if cacertPath == "" {
		return &tls.Config{}
	}

	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile(cacertPath)
	if err != nil {
		panic("Error reading CA certificate from " + cacertPath)
	}

	certpool.AppendCertsFromPEM(pemCerts)

	return &tls.Config{
		RootCAs:            certpool,
		InsecureSkipVerify: verify,
	}
}

func parseBrokerUrls(brokerUrls string) []string {
	tokens := strings.Split(brokerUrls, ",")
	for i, url := range tokens {
		tokens[i] = strings.TrimSpace(url)
	}

	return tokens
}
