package main

import (
		"encoding/json"
		"errors"
		"fmt"
		"github.com/shirou/gopsutil/mem"
		log "github.com/sirupsen/logrus"
    	"time"
)

var PluginEnv		mem.VirtualMemoryStat
var PluginConfig 	map[string]map[string]map[string]interface{}
var PluginData		map[string]interface{}


func PluginMeasure() ([]byte, []byte, float64) {
	// Get measurement of mem
	vmem, _ 					:= mem.VirtualMemory()
	PluginData["virtualmem"]	= *vmem
	// Make it understandable
	// Apply USE methodology for MEM
	// U: 	Usage (usually throughput/latency indicators)
	//		In this case we define as Available memory % 0-100
	// S:	Saturation (measured relevant to Design point)
	// E:	Errors (not applicable for MEM)
	// Prepare the data
	PluginData["mem"]    		= 100.0 * float64((PluginData["virtualmem"].(mem.VirtualMemoryStat).Total - PluginData["virtualmem"].(mem.VirtualMemoryStat).Available))/float64(PluginData["virtualmem"].(mem.VirtualMemoryStat).Total)
	PluginData["use"]    		= PluginData["mem"]
	PluginData["latency"]  		= 0.00
	PluginData["throughput"]  	= 0.00
	PluginData["throughputmax"] = 0.00
	PluginData["saturation"]    = 100.0 * PluginData["use"].(float64) / PluginConfig["alert"]["mem"]["design"].(float64)
	PluginData["errors"]    	= 0.00

	myMeasure, _				:= json.Marshal(PluginData["mem"])
	myMeasureRaw, _ 			:= json.Marshal(PluginData)
	return myMeasure, myMeasureRaw, float64(time.Now().UnixNano())/1e9
}

func PluginAlert(measure []byte) (string, string, bool, error) {
	alertMsg  := ""
	alertLvl  := ""
	alertFlag := false
	alertErr  := errors.New("nothing")

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
	vmem, _		:= mem.VirtualMemory()
	PluginEnv	= *vmem
	err := json.Unmarshal([]byte(config), &PluginConfig)
	if err != nil {
		log.WithFields(log.Fields{"config": config}).Error("failed to unmarshal config")
	}
	log.WithFields(log.Fields{"pluginconfig": PluginConfig, "pluginenv": PluginEnv }).Info("InitPlugin mem")
}


func main() {
	config  := 	`
				{"alert": { 
				            "mem": 		{"low": 10, "design": 70.0, "engineered": 90.0}
				          }
				}
				`
	InitPlugin(config)
	log.WithFields(log.Fields{"PluginConfig": PluginConfig}).Info("InitPlugin")
	tickd := 1* time.Second
	for i := 1; i <= 2; i++ {
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
