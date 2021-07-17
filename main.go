package main

import (
	"encoding/csv"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PLAISIR-FLUX/banana-vpc-csv/ftp"
)

func main() {
	for {
		<-time.After(10 * time.Second)
		go func() {
			client := &http.Client{}
			req, _ := http.NewRequest("GET", "https://b2b.banana-vpc.com/modules/customexporter/downloads/product_id_shop_1.csv", nil)
			req.Header.Add("User-Agent", "")
			resp, err := client.Do(req)
			if err != nil {
				log.Println(err)
			}
			defer resp.Body.Close()
			reader := csv.NewReader(resp.Body)
			reader.Comma = ';'
			lines, _ := reader.ReadAll()
			for i, l := range lines {
				if i > 0 {
					lines[i][5] = strings.Replace(l[5], "_", ",", -1)
					if l[9] == "En Stock" {
						lines[i][9] = "10"
					} else if l[9] == "En rupture" {
						lines[i][9] = "0"
					}
				}
			}
			f, _ := os.Create("product_id_shop_1.csv")
			defer f.Close()
			w := csv.NewWriter(f)
			w.Comma = ';'
			w.WriteAll(lines)
			config := ftp.Config{
				User:               os.Getenv("USERNAME"),
				Password:           os.Getenv("PASSWORD"),
				ConnectionsPerHost: 10,
				Timeout:            10 * time.Second,
				Logger:             os.Stderr,
			}
			ftpClient, _ := ftp.DialConfig(config, os.Getenv("HOST"))
			f, _ = os.Open("product_id_shop_1.csv")
			defer f.Close()
			ftpClient.Store("/product_id_shop_1.csv", f)
		}()
	}
}
