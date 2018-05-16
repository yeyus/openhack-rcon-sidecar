package main

import (
	"fmt"
	"github.com/andrewtian/minepong"
	//"github.com/bearbin/mcgorcon"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/kelseyhightower/envconfig"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ... {customerId} ...  {resource}
const DATA_COLLECTOR_API = "https://%s.ods.opinsights.azure.com%s?api-version=2016-04-01"
const JSON = "application/json"

type Config struct {
	PodName string `envconfig:"POD_NAME"`

	// Minecraft server info
	Host       string `envconfig:"HOST"`
	Port       string `envconfig:"PORT"`
	Password   string `envconfig:"PASSWORD"`
	DataVolume string `envconfig:"DATA_VOLUME"`

	// LogAnalytics info
	CustomerId string `envconfig:"AZURE_CUSTOMER_ID"`
	SharedKey  string `envconfig:"AZURE_SHARED_KEY"`
}

type OnlineUsersPayload struct {
	PodName       string
	OnlinePlayers int
	MaxPlayers    int
	Population    int
}

func CountFilesInDir(dir string) int {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Printf("Error counting files in %s directory:, %s", dir, err.Error())
	}
	return len(files)
}

// https://docs.microsoft.com/en-us/azure/log-analytics/log-analytics-data-collector-api#python-sample
func Sign(customerId string, sharedKey string, date string, contentLength int, method string, resource string) string {
	headers := fmt.Sprintf("x-ms-date:%s", date)
	stringToHash := fmt.Sprintf("%s\n%d\n%s\n%s\n%s", method, contentLength, JSON, headers, resource)
	bytesToHash, _ := base64.StdEncoding.DecodeString(sharedKey)
	mac := hmac.New(sha256.New, bytesToHash)
	mac.Write([]byte(stringToHash))
	expectedMAC := mac.Sum(nil)
	encodedHash := base64.StdEncoding.EncodeToString(expectedMAC)

	return fmt.Sprintf("SharedKey %s:%s", customerId, encodedHash)
}

func PostToLogAnalytics(config Config, stats OnlineUsersPayload) error {
	jsonData, err := json.Marshal(&stats)
	if err != nil {
		return err
	}

	requestDate := time.Now().UTC().Format(time.RFC1123)
	requestDate = strings.Replace(requestDate, "UTC", "GMT", 1)

	method := "POST"
	resource := "/api/logs"
	signature := Sign(config.CustomerId, config.SharedKey, requestDate, len(jsonData), method, resource)
	client := &http.Client{}
	req, err := http.NewRequest(method, fmt.Sprintf(DATA_COLLECTOR_API, config.CustomerId, resource), bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", JSON)
	req.Header.Add("Authorization", signature)
	req.Header.Add("Log-Type", "MinecraftStats")
	req.Header.Add("x-ms-date", requestDate)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Print("Request:")
	log.Printf("status=%s\n", resp.Status)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Printf("body=%s\n", string(body))

	return nil
}

func GetServerStatus(config Config) error {
	portHost := fmt.Sprintf("%s:%s", config.Host, config.Port)
	connection, err := net.Dial("tcp", portHost)
	defer connection.Close()
	if err != nil {
		return err
	}

	pong, err := minepong.Ping(connection, portHost)
	if err != nil {
		return err
	}

	count := CountFilesInDir(config.DataVolume)
	log.Printf("Server population is %d", count)

	err = PostToLogAnalytics(config, OnlineUsersPayload{
		PodName:       config.PodName,
		OnlinePlayers: pong.Players.Online,
		MaxPlayers:    pong.Players.Max,
		Population:    count,
	})
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", pong)

	return nil
}

func main() {
	var config Config
	err := envconfig.Process("rcon", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Handle SIGTERM signal
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range c {
			log.Print("Detected exit signal")
			os.Exit(0)
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				err := GetServerStatus(config)
				if err != nil {
					log.Printf("Error while checking server status: %s", err.Error())
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	// run forever until signal
	select {}
}
