package speedtest

import (
	"context"
	"errors"
	"fmt"
	"github.com/justhx0r/speedtest-go/speedtest/tcp"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type (
	downloadFunc func(context.Context, *Server, int) error
	uploadFunc   func(context.Context, *Server, int) error
)

var (
	dlSizes = [...]int{350, 500, 750, 1000, 1500, 2000, 2500, 3000, 3500, 4000}
	ulSizes = [...]int{100, 300, 500, 800, 1000, 1500, 2500, 3000, 3500, 4000} // kB
)

var (
	ErrConnectTimeout = errors.New("server connect timeout")
)

func (s *Server) MultiDownloadTestContext(ctx context.Context, servers Servers) error {
	if s.Context.config.NoDownload {
		dbg.Println("Download test disabled")
		return nil
	}
	ss := servers.Available()
	if ss.Len() == 0 {
		return errors.New("not found available servers")
	}
	mainIDIndex := 0
	var fp *funcGroup
	_context, cancel := context.WithCancel(ctx)
	for i, server := range *ss {
		if server.ID == s.ID {
			mainIDIndex = i
		}
		sp := server
		dbg.Printf("Register Download Handler: %s\n", sp.URL)
		fp = server.Context.RegisterDownloadHandler(func() {
			_ = downloadRequest(_context, sp, 3)
		})
	}
	fp.Start(cancel, mainIDIndex) // block here
	s.DLSpeed = fp.manager.GetAvgDownloadRate()
	return nil
}

func (s *Server) MultiUploadTestContext(ctx context.Context, servers Servers) error {
	if s.Context.config.NoUpload {
		dbg.Println("Upload test disabled")
		return nil
	}
	ss := servers.Available()
	if ss.Len() == 0 {
		return errors.New("not found available servers")
	}
	mainIDIndex := 0
	var fp *funcGroup
	_context, cancel := context.WithCancel(ctx)
	for i, server := range *ss {
		if server.ID == s.ID {
			mainIDIndex = i
		}
		sp := server
		dbg.Printf("Register Upload Handler: %s\n", sp.URL)
		fp = server.Context.RegisterUploadHandler(func() {
			_ = uploadRequest(_context, sp, 3)
		})
	}
	fp.Start(cancel, mainIDIndex) // block here
	s.ULSpeed = fp.manager.GetAvgUploadRate()
	return nil
}

// DownloadTest executes the test to measure download speed
func (s *Server) DownloadTest() error {
	return s.downloadTestContext(context.Background(), downloadRequest)
}

// DownloadTestContext executes the test to measure download speed, observing the given context.
func (s *Server) DownloadTestContext(ctx context.Context) error {
	return s.downloadTestContext(ctx, downloadRequest)
}

func (s *Server) downloadTestContext(ctx context.Context, downloadRequest downloadFunc) error {
	if s.Context.config.NoDownload {
		dbg.Println("Download test disabled")
		return nil
	}
	start := time.Now()
	_context, cancel := context.WithCancel(ctx)
	s.Context.RegisterDownloadHandler(func() {
		_ = downloadRequest(_context, s, 3)
	}).Start(cancel, 0)
	duration := time.Since(start)
	s.DLSpeed = s.Context.GetAvgDownloadRate()
	s.TestDuration.Download = &duration
	s.testDurationTotalCount()
	return nil
}

// UploadTest executes the test to measure upload speed
func (s *Server) UploadTest() error {
	return s.uploadTestContext(context.Background(), uploadRequest)
}

// UploadTestContext executes the test to measure upload speed, observing the given context.
func (s *Server) UploadTestContext(ctx context.Context) error {
	return s.uploadTestContext(ctx, uploadRequest)
}

func (s *Server) uploadTestContext(ctx context.Context, uploadRequest uploadFunc) error {
	if s.Context.config.NoUpload {
		dbg.Println("Upload test disabled")
		return nil
	}
	start := time.Now()
	_context, cancel := context.WithCancel(ctx)
	s.Context.RegisterUploadHandler(func() {
		_ = uploadRequest(_context, s, 4)
	}).Start(cancel, 0)
	duration := time.Since(start)
	s.ULSpeed = s.Context.GetAvgUploadRate()
	s.TestDuration.Upload = &duration
	s.testDurationTotalCount()
	return nil
}

func downloadRequest(ctx context.Context, s *Server, w int) error {
	size := dlSizes[w]
	xdlURL := strings.Split(s.URL, "/upload.php")[0] + "/random" + strconv.Itoa(size) + "x" + strconv.Itoa(size) + ".jpg"
	dbg.Printf("XdlURL: %s\n", xdlURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xdlURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.Context.doer.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return s.Context.NewChunk().DownloadHandler(resp.Body)
}

func uploadRequest(ctx context.Context, s *Server, w int) error {
	size := ulSizes[w]
	dc := s.Context.NewChunk().UploadHandler(int64(size*100-51) * 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, dc)
	req.ContentLength = dc.(*DataChunk).ContentLength
	dbg.Printf("Len=%d, XulURL: %s\n", req.ContentLength, s.URL)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.Context.doer.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return err
}

// PingTest executes test to measure latency
func (s *Server) PingTest(callback func(latency time.Duration)) error {
	return s.PingTestContext(context.Background(), callback)
}

// PingTestContext executes test to measure latency, observing the given context.
func (s *Server) PingTestContext(ctx context.Context, callback func(latency time.Duration)) (err error) {
	start := time.Now()
	var vectorPingResult []int64
	if s.Context.config.PingMode == TCP {
		vectorPingResult, err = s.TCPPing(ctx, 10, time.Millisecond*200, callback)
	} else if s.Context.config.PingMode == ICMP {
		vectorPingResult, err = s.ICMPPing(ctx, time.Second*4, 10, time.Millisecond*200, callback)
	} else {
		vectorPingResult, err = s.HTTPPing(ctx, 10, time.Millisecond*200, callback)
	}
	if err != nil || len(vectorPingResult) == 0 {
		return err
	}
	dbg.Printf("Before StandardDeviation: %v\n", vectorPingResult)
	mean, _, std, min, max := StandardDeviation(vectorPingResult)
	duration := time.Since(start)
	s.Latency = time.Duration(mean) * time.Nanosecond
	s.Jitter = time.Duration(std) * time.Nanosecond
	s.MinLatency = time.Duration(min) * time.Nanosecond
	s.MaxLatency = time.Duration(max) * time.Nanosecond
	s.TestDuration.Ping = &duration
	s.testDurationTotalCount()
	return nil
}

// TestAll executes ping, download and upload tests one by one
func (s *Server) TestAll() error {
	err := s.PingTest(nil)
	if err != nil {
		return err
	}
	err = s.DownloadTest()
	if err != nil {
		return err
	}
	return s.UploadTest()
}

func (s *Server) TCPPing(
	ctx context.Context,
	echoTimes int,
	echoFreq time.Duration,
	callback func(latency time.Duration),
) (latencies []int64, err error) {
	var pingDst string
	if len(s.Host) == 0 {
		u, err := url.Parse(s.URL)
		if err != nil || len(u.Host) == 0 {
			return nil, err
		}
		pingDst = u.Host
	} else {
		pingDst = s.Host
	}
	failTimes := 0
	client := tcp.NewClient(s.Context.tcpDialer, pingDst)
	err = client.Connect()
	if err != nil {
		return nil, err
	}
	for i := 0; i < echoTimes; i++ {
		latency, err := client.PingContext(ctx)
		if err != nil {
			failTimes++
			continue
		}
		latencies = append(latencies, latency)
		if callback != nil {
			callback(time.Duration(latency))
		}
		time.Sleep(echoFreq)
	}
	if failTimes == echoTimes {
		return nil, ErrConnectTimeout
	}
	return
}

func (s *Server) HTTPPing(
	ctx context.Context,
	echoTimes int,
	echoFreq time.Duration,
	callback func(latency time.Duration),
) (latencies []int64, err error) {
	u, err := url.Parse(s.URL)
	if err != nil || len(u.Host) == 0 {
		return nil, err
	}
	pingDst := fmt.Sprintf("%s/latency.txt", s.URL)
	dbg.Printf("Echo: %s\n", pingDst)
	failTimes := 0
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingDst, nil)
	if err != nil {
		return nil, err
	}
	for i := 0; i < echoTimes; i++ {
		sTime := time.Now()
		_, err = s.Context.doer.Do(req)
		endTime := time.Since(sTime)
		if err != nil {
			failTimes++
			continue
		}
		latencies = append(latencies, endTime.Nanoseconds()/2)
		dbg.Printf("2RTT: %s\n", endTime)
		if callback != nil {
			callback(endTime / 2)
		}
		time.Sleep(echoFreq)
	}
	if failTimes == echoTimes {
		return nil, ErrConnectTimeout
	}
	return
}

const PingTimeout = -1
const echoOptionDataSize = 32 // `echoMessage` need to change at same time

// ICMPPing privileged method
func (s *Server) ICMPPing(
	ctx context.Context,
	readTimeout time.Duration,
	echoTimes int,
	echoFreq time.Duration,
	callback func(latency time.Duration),
) (latencies []int64, err error) {
	u, err := url.ParseRequestURI(s.URL)
	if err != nil || len(u.Host) == 0 {
		return nil, err
	}
	dbg.Printf("Echo: %s\n", strings.Split(u.Host, ":")[0])
	dialContext, err := s.Context.ipDialer.DialContext(ctx, "ip:icmp", strings.Split(u.Host, ":")[0])
	if err != nil {
		return nil, err
	}
	defer dialContext.Close()

	ICMPData := make([]byte, 8+echoOptionDataSize) // header + data
	ICMPData[0] = 8                                // echo
	ICMPData[1] = 0                                // code
	ICMPData[2] = 0                                // checksum
	ICMPData[3] = 0                                // checksum
	ICMPData[4] = 0                                // id
	ICMPData[5] = 1                                // id
	ICMPData[6] = 0                                // seq
	ICMPData[7] = 1                                // seq

	var echoMessage = "Hi! SpeedTest-Go \\(●'◡'●)/"

	for i := 0; i < len(echoMessage); i++ {
		ICMPData[7+i] = echoMessage[i]
	}

	failTimes := 0
	for i := 0; i < echoTimes; i++ {
		ICMPData[2] = byte(0)
		ICMPData[3] = byte(0)

		ICMPData[6] = byte(1 >> 8)
		ICMPData[7] = byte(1)
		ICMPData[8+echoOptionDataSize-1] = 6
		cs := checkSum(ICMPData)
		ICMPData[2] = byte(cs >> 8)
		ICMPData[3] = byte(cs)

		sTime := time.Now()
		_ = dialContext.SetDeadline(sTime.Add(readTimeout))
		_, err = dialContext.Write(ICMPData)
		if err != nil {
			failTimes += echoTimes - i
			break
		}
		buf := make([]byte, 20+echoOptionDataSize+8)
		_, err = dialContext.Read(buf)
		if err != nil || buf[20] != 0x00 {
			failTimes++
			continue
		}
		endTime := time.Since(sTime)
		latencies = append(latencies, endTime.Nanoseconds())
		dbg.Printf("1RTT: %s\n", endTime)
		if callback != nil {
			callback(endTime)
		}
		time.Sleep(echoFreq)
	}
	if failTimes == echoTimes {
		return nil, ErrConnectTimeout
	}
	return
}

func checkSum(data []byte) uint16 {
	var sum uint32
	var length = len(data)
	var index int
	for length > 1 {
		sum += uint32(data[index])<<8 + uint32(data[index+1])
		index += 2
		length -= 2
	}
	if length > 0 {
		sum += uint32(data[index])
	}
	sum += sum >> 16
	return uint16(^sum)
}

func StandardDeviation(vector []int64) (mean, variance, stdDev, min, max int64) {
	if len(vector) == 0 {
		return
	}
	var sumNum, accumulate int64
	min = math.MaxInt64
	max = math.MinInt64
	for _, value := range vector {
		sumNum += value
		if min > value {
			min = value
		}
		if max < value {
			max = value
		}
	}
	mean = sumNum / int64(len(vector))
	for _, value := range vector {
		accumulate += (value - mean) * (value - mean)
	}
	variance = accumulate / int64(len(vector))
	stdDev = int64(math.Sqrt(float64(variance)))
	return
}
