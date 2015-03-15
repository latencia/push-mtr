package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	log "github.com/Sirupsen/logrus"
	"github.com/grindhold/gominatim"
	geoipc "github.com/rubiojr/freegeoip-client"
	"gopkg.in/alecthomas/kingpin.v1"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	mqttClient *mqtt.MqttClient
)

type Host struct {
	IP          string  `json:"ip"`
	Name        string  `json:"hostname"`
	Hop         int     `json:"hop-number"`
	Sent        int     `json:"sent"`
	LostPercent float64 `json:"lost-percent"`
	Last        float64 `json:"last"`
	Avg         float64 `json:"avg"`
	Best        float64 `json:"best"`
	Worst       float64 `json:"worst"`
	StDev       float64 `json:"standard-dev"`
}

type Report struct {
	Time        time.Time       `json:"time"`
	Hosts       []*Host         `json:"hosts"`
	Hops        int             `json:"hops"`
	ElapsedTime time.Duration   `json:"elapsed_time"`
	Location    *ReportLocation `json:"location"`
}

// slightly simpler struct than the one provided by geoipc
type ReportLocation struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

func NewReport(reportCycles int, host string, loc *ReportLocation, args ...string) *Report {
	report := &Report{}
	report.Time = time.Now()
	args = append([]string{"--report", "-n", "-c", strconv.Itoa(reportCycles), host}, args...)

	tstart := time.Now()
	mtr := findMtrBin()
	rawOutput, err := exec.Command(mtr, args...).Output()

	if err != nil {
		panic("Error running the mtr command")
	}

	buf := bytes.NewBuffer(rawOutput)
	scanner := bufio.NewScanner(buf)
	scanner.Split(bufio.ScanLines)
	hopCount := 0

	for scanner.Scan() {
		r, _ := regexp.Compile(`^\s+\d+\.`)

		line := scanner.Text()
		if !r.MatchString(line) {
			continue
		}

		tokens := strings.Fields(line)
		sent, err := strconv.Atoi(tokens[3])
		if err != nil {
			panic("Error parsing sent field")
		}

		hopCount += 1
		host := Host{
			IP:   tokens[1],
			Sent: sent,
			Hop:  hopCount,
		}

		f2F(strings.Replace(tokens[2], "%", "", -1), &host.LostPercent)
		f2F(tokens[4], &host.Last)
		f2F(tokens[5], &host.Avg)
		f2F(tokens[6], &host.Best)
		f2F(tokens[7], &host.Worst)
		f2F(tokens[8], &host.StDev)

		report.Hosts = append(report.Hosts, &host)
	}

	report.Hops = len(report.Hosts)
	report.ElapsedTime = time.Since(tstart)
	report.Location = loc

	return report
}

func f2F(val string, field *float64) {
	f, err := strconv.ParseFloat(val, 64)
	*field = f
	if err != nil {
		panic("Error parsing field")
	}
}

func findMtrBin() string {
	paths := os.Getenv("PATH")
	if paths == "" {
		return ""
	}

	for _, path := range strings.Split(paths, ":") {
		if _, err := os.Stat(path + "/mtr"); err == nil {
			return path + "/mtr"
		}
	}

	return ""
}

func urlGet(url string, topic string) (err error) {
	var testResult WgetResult

	if testResult, err = Wget(url, "", true); err != nil {
		return fmt.Errorf("Error getting download URL metrics: %s\n", err)
	}
	fmt.Println(testResult)

	msg, _ := json.MarshalIndent(testResult, "", "  ")
	log.Debugf("Sending URL Get report to %s", topic)
	fmt.Println(string(msg))
	pushMsg(topic, string(msg))

	return nil
}

func run(count int, host string, loc *ReportLocation, stdout bool, topic string) (err error) {
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

func parseBrokerUrls(brokerUrls string) []string {
	tokens := strings.Split(brokerUrls, ",")
	for i, url := range tokens {
		tokens[i] = strings.TrimSpace(url)
	}

	return tokens
}

func handleError(err error, fail bool) {
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error()+"\n")
	}

	if fail {
		os.Exit(1)
	}
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

	clientID := kingpin.Flag("clientid", "Use a custom MQTT client ID").String()

	furlGet := kingpin.Flag("url-get", "Report URL GET metrics").String()

	kingpin.Parse()

	log.Info("Starting push-mtr")

	var err error

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	go func() {
		fmt.Println(http.ListenAndServe("0.0.0.0:6161", nil))
	}()

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

	if findMtrBin() == "" {
		fmt.Fprintf(os.Stderr, "mtr binary not found in path\n")
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

	errorChan := make(chan error)
	runUrlGet := func(scheme *string, host *string) {
		if *scheme != "" {
			go func() {
				if err := urlGet(*scheme+"://"+*host, *urlGetTopic); err != nil {
					errorChan <- err
				}
			}()
			select {
			case err := <-errorChan:
				log.Error(err)
			default:
			}
		}
	}

	if *repeat != 0 {
		timer := time.NewTicker(time.Duration(*repeat) * time.Second)
		for _ = range timer.C {
			runUrlGet(furlGet, host)
			err = run(*count, *host, loc, *stdout, *topic)
			handleError(err, false)
		}
	} else {
		runUrlGet(furlGet, host)
		err := run(*count, *host, loc, *stdout, *topic)
		handleError(err, true)
	}
}

func pushMsg(topic, msg string) {
	<-mqttClient.Publish(mqtt.QOS_ONE, topic, msg)
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

func geoipLoc() chan ReportLocation {
	iplocChan := make(chan ReportLocation)
	go func() {
		// retry once
		for i := 0; i < 2; i++ {
			res, err := geoipc.GetLocation()
			if err == nil {
				loc := ReportLocation{
					CountryCode: strings.ToLower(res.CountryCode), // normalize code
					CountryName: res.CountryName,
					City:        res.City,
					Latitude:    res.Latitude,
					Longitude:   res.Longitude,
					IP:          res.IP,
				}
				iplocChan <- loc
				break
			}
			time.Sleep(2 * time.Second)
		}
		close(iplocChan)
	}()

	return iplocChan
}

func nominatimLoc(query string) chan ReportLocation {
	gominatim.SetServer("https://nominatim.openstreetmap.org/")

	ch := make(chan ReportLocation, 1)
	go func(query string) {
		qry := gominatim.SearchQuery{
			Q:              query,
			Addressdetails: true,
			Limit:          1,
			AcceptLanguage: "en-US",
		}

		for i := 0; i < 2; i++ {
			res, err := qry.Get()
			if err == nil && len(res) > 0 {
				res1 := res[0]
				lat, _ := strconv.ParseFloat(res1.Lat, 64)
				lon, _ := strconv.ParseFloat(res1.Lon, 64)
				loc := ReportLocation{
					CountryCode: res1.Address.CountryCode,
					CountryName: res1.Address.Country,
					City:        res1.Address.City,
					Latitude:    lat,
					Longitude:   lon,
				}
				ch <- loc
				break
			}
			time.Sleep(2 * time.Second)
		}
		close(ch)
	}(query)

	return ch
}

func findLocation(query string) (*ReportLocation, error) {

	if query == "" {

		chan1 := geoipLoc()
		loc := <-chan1
		return &loc, nil

	} else {

		chan1 := geoipLoc()
		chan2 := nominatimLoc(query)

		iploc := <-chan1
		nominatimLoc := <-chan2

		nominatimLoc.IP = iploc.IP

		return &nominatimLoc, nil
	}

}

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
