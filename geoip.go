package main

import (
	"github.com/grindhold/gominatim"
	geoipc "github.com/rubiojr/freegeoip-client"
	"strconv"
	"strings"
	"time"
)

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
