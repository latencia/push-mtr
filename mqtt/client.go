package mqtt

import (
	"errors"
	"fmt"
	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	"net/url"
	"time"
)

// tcp://user:password@host:port
func PushMsg(clientId, brokerUrl, topic, msg string) error {

	if brokerUrl == "" {
		panic("Invalid broker URL")
	}

	uri, _ := url.Parse(brokerUrl)
	if uri.Scheme != "tcp" {
		panic("Invalid broker URL scheme")
	}

	opts := mqtt.NewClientOptions()
	opts.SetCleanSession(true)
	opts.SetWriteTimeout(10 * time.Second)

	opts.AddBroker(fmt.Sprintf("tcp://%s", uri.Host))

	if uri.User != nil {
		user := uri.User.Username()
		opts.SetUsername(user)
		password, _ := uri.User.Password()
		if password != "" {
			opts.SetPassword(password)
		}
	}

	opts.SetClientId(clientId)

	client := mqtt.NewClient(opts)
	_, err := client.Start()
	if err != nil {
		return errors.New("Error starting the MQTT Client: " + err.Error())
	}

	<-client.Publish(0, topic, msg)
	client.Disconnect(0)

	return nil
}

func PushMsgMulti(clientId string, brokerUrls []string, topic, msg string) (string, []error) {
	errs := make([]error, len(brokerUrls))
	for i, broker := range brokerUrls {
		err := PushMsg(clientId, broker, topic, msg)
		if err == nil { return broker, nil }
		errs[i] = err
	}
	return "", errs
}
