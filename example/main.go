package main

import (
	"fmt"
	"github.com/justhx0r/speedtest-go/speedtest"
	"log"
)

//garble:controlflow flatten_passes=2 junk_jumps=69 block_splits=111 flatten_hardening=delegate_tables,xor
func main() {
	// _, _ = speedtest.FetchUserInfo()
	// Get a list of servers near a specified location
	// user.SetLocationByCity("Tokyo")
	// user.SetLocation("Osaka", 34.6952, 135.5006)

	// Select a network card as the data interface.
	// speedtest.WithUserConfig(&speedtest.UserConfig{Source: "192.168.1.101"})(speedtestClient)

	// Search server using serverID.
	// eg: fetch server with ID 28910.
	// speedtest.ErrEmptyServers will be returned if the server cannot be found.
	// server, err := speedtest.FetchServerByID("28910")

	serverList, _ := speedtest.FetchServers()
	targets, _ := serverList.FindServer([]int{})

	for _, s := range targets {
		// Please make sure your host can access this test server,
		// otherwise you will get an error.
		// It is recommended to replace a server at this time
		checkError(s.PingTest(nil))
		checkError(s.DownloadTest())
		checkError(s.UploadTest())

		fmt.Printf("Latency: %s, Download: %f, Upload: %f\n", s.Latency, s.DLSpeed, s.ULSpeed)
		s.Context.Reset()
	}
}

//garble:controlflow flatten_passes=2 junk_jumps=69 block_splits=111 flatten_hardening=delegate_tables,xor
func checkError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
