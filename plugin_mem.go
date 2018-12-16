package main

import (
		"encoding/json"
		"errors"
		"fmt"
		"github.com/shirou/gopsutil/mem"
		log "github.com/sirupsen/logrus"
		"github.com/prometheus/client_golang/prometheus"
		"github.com/prometheus/client_golang/prometheus/promhttp"
		"net/http"
    	"time"
)

var PluginConfig 	map[string]map[string]map[string]interface{}
var PluginData		map[string]interface{}

//	Define the metrics we wish to expose
var memIndicator = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "sreagent_mem_metrics",
		Help: "MEM Load Utilization Saturation Errors Throughput Latency",
	}, []string{"use"} )

var memUsage = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "sreagent_mem_usage",
		Help: "MEM usage details",
	}, []string{"mem"} )


func PluginMeasure() ([]byte, []byte, float64) {
	// Get measurement of mem
	vmem, _ 					:= mem.VirtualMemory()
	PluginData["virtualmem"]	= *vmem
	// Make it understandable
	// Apply USE methodology for MEM
	// U: 	Usage (usually throughput/latency indicators)
	//		In this case we define as Available memory % 0-100
	// S:	Saturation (paging)
	// E:	Errors (not applicable for MEM)
	// Prepare the data
	PluginData["mem"]    		= 100.0 * float64((PluginData["virtualmem"].(mem.VirtualMemoryStat).Total - PluginData["virtualmem"].(mem.VirtualMemoryStat).Available))/float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Total)
	PluginData["use"]    		= PluginData["mem"]
	PluginData["latency"]  		= 0.00
	PluginData["throughput"]  	= 0.00
	PluginData["throughputmax"] = 0.00
	PluginData["saturation"]    = 100.0 * float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).SwapCached)/float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Total)
	PluginData["errors"]    	= 0.00

	// Update metrics related to the plugin
	memUsage.With(prometheus.Labels{"mem":  "memtotal"}).Set( float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Total)/1024.0 )
	memUsage.With(prometheus.Labels{"mem":  "available"}).Set( float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Available)/1024.0 )
	memUsage.With(prometheus.Labels{"mem":  "free"}).Set( float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Free)/1024.0 )
	memUsage.With(prometheus.Labels{"mem":  "used"}).Set( float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Used)/1024.0 )
	memUsage.With(prometheus.Labels{"mem":  "buffers"}).Set( float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Buffers)/1024.0 )
	memUsage.With(prometheus.Labels{"mem":  "cached"}).Set( float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Cached)/1024.0 )

	memIndicator.With(prometheus.Labels{"use":  "utilization"}).Set(PluginData["use"].(float64))
	memIndicator.With(prometheus.Labels{"use":  "saturation"}).Set(PluginData["saturation"].(float64))
	memIndicator.With(prometheus.Labels{"use":  "throughput"}).Set(PluginData["throughput"].(float64))
	memIndicator.With(prometheus.Labels{"use":  "errors"}).Set(PluginData["errors"].(float64))

	myMeasure, _				:= json.Marshal(PluginData)
	return myMeasure, []byte{}, float64(time.Now().UnixNano())/1e9
}

func PluginAlert(measure []byte) (string, string, bool, error) {
	alertMsg  := ""
	alertLvl  := ""
	alertFlag := false
	alertErr  := errors.New("no error")

	// Check that the mem overall value is within range
	switch {
		case PluginData["mem"].(float64) < PluginConfig["alert"]["mem"]["low"].(float64):
			alertLvl  = "warn"
			alertMsg  += "Overall mem below low design point "
			alertFlag = true
			alertErr  = errors.New("low mem")
		case PluginData["mem"].(float64) > PluginConfig["alert"]["mem"]["engineered"].(float64):
			alertLvl  = "fatal"
			alertMsg  += "Overall mem above engineered point "
			alertFlag = true
			alertErr  = errors.New("excessive mem")
			// return now, looks bad
			return alertMsg, alertLvl, alertFlag, alertErr
		case PluginData["mem"].(float64) > PluginConfig["alert"]["mem"]["design"].(float64):
			alertLvl  = "warn"
			alertMsg  += "Overall mem above design point "
			alertFlag = true
			alertErr  = errors.New("moderately high mem")
	}
	return alertMsg, alertLvl, alertFlag, alertErr
}


func InitPlugin(config string) () {
	if PluginData == nil {
		PluginData = make(map[string]interface{},20)
	}
	if PluginConfig == nil {
		PluginConfig = make(map[string]map[string]map[string]interface{},20)
	}

	// Register metrics with prometheus
	prometheus.MustRegister(memIndicator)
	prometheus.MustRegister(memUsage)

	err := json.Unmarshal([]byte(config), &PluginConfig)
	if err != nil {
		log.WithFields(log.Fields{"config": config}).Error("failed to unmarshal config")
	}
	log.WithFields(log.Fields{"pluginconfig": PluginConfig}).Info("InitPlugin mem")
}


func main() {
	config  := 	`
				{"alert": { 
				            "mem": 		{"low": 10, "design": 70.0, "engineered": 90.0}
				          }
				}
				`

	//--------------------------------------------------------------------------//
	// time to start a prometheus metrics server
	// and export any metrics on the /metrics endpoint.
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.ListenAndServe(":8999", nil)
	}()
	//--------------------------------------------------------------------------//

	InitPlugin(config)
	log.WithFields(log.Fields{"PluginConfig": PluginConfig}).Info("InitPlugin")
	tickd := 1* time.Second
	for i := 1; i <= 20; i++ {
		tick := time.Now().UnixNano()
		measure, measureraw, measuretimestamp := PluginMeasure()
		alertmsg, alertlvl, isAlert, err := PluginAlert(measure)
		fmt.Printf("Iteration #%d tick %d \n", i, tick)
		log.WithFields(log.Fields{"timestamp": measuretimestamp, 
					  "measure": string(measure[:]),
					  "measureraw": string(measureraw[:]),
					  "PluginData": PluginData,
					  "alertMsg": alertmsg,
					  "alertLvl": alertlvl,
					  "isAlert":  isAlert,
					  "AlertErr":      err,
		}).Info("Tick")
		time.Sleep(tickd)
	}
}
