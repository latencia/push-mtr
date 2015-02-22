package main

import (
	mqttc "./mqttc"
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	geoipc "github.com/rubiojr/freegeoip-client"
	"gopkg.in/alecthomas/kingpin.v1"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Host struct {
	IP          string  `json:"ip"`
	Name        string  `json:"hostname"`
	Hop         int     `json:"hop-number"`
	Sent        int     `json:"sent"`
	LostPercent float64 `json:"lost-percent"`
	Last        float64 `json:"mean"`
	Avg         float64 `json:"mean"`
	Best        float64 `json:"best"`
	Worst       float64 `json:"worst"`
	StDev       float64 `json:"standard-dev"`
}

type Report struct {
	Time        time.Time       `json:"time"`
	Hosts       []*Host         `json:"hosts"`
	Hops        int             `json:"hops"`
	ElapsedTime time.Duration   `json:"elapsed_time"`
	Location    geoipc.Location `json:"location"`
}

func NewReport(reportCycles int, host string, args ...string) *Report {
	loc, err := geoipc.GetLocation()
	if err != nil {
		log.Errorf("Error getting location from geoip server: %s", err)
		loc = geoipc.Location{}
	}

	return NewReportWithLoc(reportCycles, host, &loc, args...)
}

func NewReportWithLoc(reportCycles int, host string, loc *geoipc.Location, args ...string) *Report {
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

		host := Host{
			IP:   tokens[1],
			Sent: sent,
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
	report.Location = *loc

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

func run(count int, host string, stdout bool, args *mqttc.Args) error {
	r := NewReport(count, host)

	var err error = nil
	if stdout {
		msg, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(msg))
	} else {
		msg, _ := json.Marshal(r)
		err = mqttc.PushMsg(string(msg), args)
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
	kingpin.Version("0.2")

	count := kingpin.Flag("count", "Report cycles (mtr -c)").
		Default("10").Int()

	topic := kingpin.Flag("topic", "MTTQ topic").Default("/metrics/mtr").
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

	country := kingpin.Flag("country", "Force country (2 letter country code)").
		String()

	insecure := kingpin.Flag("insecure", "Don't verify the server's certificate chain and host name.").
		Default("false").Bool()

	kingpin.Parse()

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

	if *country != "" {
		loc := countryLoc(*country)
		if loc == nil {
			log.Fatalf("Country %s not found!", country)
			os.Exit(1)
		}
	}

	urlList := parseBrokerUrls(*brokerUrls)

	var err error
	args := mqttc.Args{
		BrokerURLs:    urlList,
		ClientID:      "push-mtr",
		Topic:         *topic,
		TLSCACertPath: *cafile,
		TLSSkipVerify: *insecure,
	}

	if *repeat != 0 {
		timer := time.NewTicker(time.Duration(*repeat) * time.Second)
		for _ = range timer.C {
			err = run(*count, *host, *stdout, &args)
			handleError(err, false)
		}
	} else {
		err := run(*count, *host, *stdout, &args)
		handleError(err, true)
	}

}

func countryLoc(code string) *geoipc.Location {
	asset, err := Asset("data/countries.csv")
	if err != nil {
		log.Panicf("Error reading country data: %s", err)
	}

	buf := bytes.NewBuffer(asset)
	reader := csv.NewReader(buf)
	records, err := reader.ReadAll()
	for _, rec := range records {
		if rec[0] == strings.ToUpper(code) {
			lat, _ := strconv.ParseFloat(rec[1], 32)
			lon, _ := strconv.ParseFloat(rec[2], 32)
			loc := &geoipc.Location{
				CountryCode: strings.ToLower(code),
				CountryName: rec[3],
				Latitude:    lat,
				Longitude:   lon,
			}
			return loc
		}
	}

	return nil
}
