package main

import (
	mqttc "./mqtt"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
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
	loc, err := geoipc.GetLocation()
	if err != nil {
		report.Location = geoipc.Location{}
	} else {
		report.Location = loc
	}

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

func run(count int, host, brokerUrl, topic string, stdout bool) error {
	r := NewReport(count, host)

	if stdout {
		msg, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(msg))
		return nil
	} else {
		msg, _ := json.Marshal(r)
		err := mqttc.PushMsg("push-mtr", brokerUrl, topic, string(msg))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error sending report: %s\n", err)
		}
		return err
	}
}

func main() {
	kingpin.Version("0.1.1")
	count := kingpin.Flag("count", "Report cycles (mtr -c)").
		Default("10").Int()

	topic := kingpin.Flag("topic", "MTTQ topic").Default("/metrics/mtr").
		String()

	host := kingpin.Arg("host", "Target host").Required().String()

	repeat := kingpin.Flag("repeat", "Send the report every X seconds").
		Default("0").Int()

	brokerUrl := kingpin.Flag("broker-url", "MQTT broker URL").
		Default("").String()

	stdout := kingpin.Flag("stdout", "Print the report to stdout").
		Default("false").Bool()

	kingpin.Parse()

	if findMtrBin() == "" {
		fmt.Fprintf(os.Stderr, "mtr binary not found in path\n")
		os.Exit(1)
	}

	if *brokerUrl == "" {
		*brokerUrl = os.Getenv("MQTT_URL")
		if *brokerUrl == "" {
			fmt.Fprintf(os.Stderr, "Invalid MQTT broker URL\n")
			os.Exit(1)
		}
	}

	if *repeat != 0 {
		timer := time.NewTicker(time.Duration(*repeat) * time.Second)
		for _ = range timer.C {
			run(*count, *host, *brokerUrl, *topic, *stdout)
		}
	} else {
		err := run(*count, *host, *brokerUrl, *topic, *stdout)
		if err != nil {
			os.Exit(1)
		}
	}
}
