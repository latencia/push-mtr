package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	"io/ioutil"
	"net/url"
	"os"
	"time"
)

type Msg struct {
	ClientID      string
	BrokerURLs    []string
	Topic         string
	Body          string
	TLSCACertPath string
	TLSVerify     bool
}

// tcp://user:password@host:port
func PushMsg(msg Msg) error {

	opts := mqtt.NewClientOptions()
	opts.SetCleanSession(true)
	opts.SetWriteTimeout(10 * time.Second)

	opts.SetClientId(msg.ClientID)
	for _, broker := range msg.BrokerURLs {
		uri, err := url.Parse(broker)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing broker url (ignored): %s\n", broker)
			continue
		}
		opts.AddBroker(broker)
		if uri.Scheme == "ssl" {
			tlsconfig := newTlsConfig(msg.TLSCACertPath, msg.TLSVerify)
			opts.SetTlsConfig(tlsconfig)
		}
	}

	client := mqtt.NewClient(opts)
	_, err := client.Start()
	if err != nil {
		return errors.New("Connection to the broker(s) failed: " + err.Error())
	}
	defer client.Disconnect(0)

	<-client.Publish(mqtt.QOS_ONE, msg.Topic, msg.Body)

	return nil
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
