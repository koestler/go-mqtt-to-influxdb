package config

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
	"os"
)

const NameRegexp = "^[a-zA-Z0-9\\-]{1,32}$"

var nameMatcher = regexp.MustCompile(NameRegexp)

func ReadConfigFile(exe, source string) (config Config, err []error) {
	yamlStr, e := ioutil.ReadFile(source)
	if e != nil {
		return config, []error{fmt.Errorf("cannot read configuration: %v; use see `%s --help`", err, exe)}
	}

	return ReadConfig(yamlStr)
}

func ReadConfig(yamlStr []byte) (config Config, err []error) {
	var configRead configRead

	yamlStr = []byte(os.ExpandEnv(string(yamlStr)))
	e := yaml.Unmarshal(yamlStr, &configRead)
	if e != nil {
		return config, []error{fmt.Errorf("cannot parse yaml: %s", err)}
	}

	return configRead.TransformAndValidate()
}

func (c Config) PrintConfig() (err error) {
	newYamlStr, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("cannot encode yaml again: %s", err)
	}

	log.Print("config: use the following config:")
	for _, line := range strings.Split(string(newYamlStr), "\n") {
		log.Print("config: ", line)
	}
	return nil
}

func (c configRead) TransformAndValidate() (ret Config, err []error) {
	var e []error
	ret.MqttClients, e = c.MqttClients.TransformAndValidate()
	err = append(err, e...)

	ret.InfluxClients, e = c.InfluxClients.TransformAndValidate()
	err = append(err, e...)

	ret.Converters, e = c.Converters.TransformAndValidate(ret.MqttClients, ret.InfluxClients)
	err = append(err, e...)

	ret.HttpServer, e = c.HttpServer.TransformAndValidate()
	err = append(err, e...)

	ret.Statistics, e = c.Statistics.TransformAndValidate()
	err = append(err, e...)

	if c.Version == nil {
		err = append(err, fmt.Errorf("Version must be defined. Use Version=0."))
	} else {
		ret.Version = *c.Version
		if ret.Version != 0 {
			err = append(err, fmt.Errorf("Version=%d is not supported.", ret.Version))
		}
	}

	if c.LogConfig != nil && *c.LogConfig {
		ret.LogConfig = true
	}

	if c.LogWorkerStart != nil && *c.LogWorkerStart {
		ret.LogWorkerStart = true
	}

	if c.LogMqttDebug != nil && *c.LogMqttDebug {
		ret.LogMqttDebug = true
	}

	return
}

func (c *httpServerConfigRead) TransformAndValidate() (ret HttpServerConfig, err []error) {
	ret.enabled = false
	ret.bind = "[::1]"
	ret.port = 8042

	if c == nil {
		return
	}

	ret.enabled = true

	if len(c.Bind) > 0 {
		ret.bind = c.Bind
	}

	if c.Port != nil {
		ret.port = *c.Port
	}

	if c.LogRequests != nil && *c.LogRequests {
		ret.logRequests = true
	}

	return
}

func (c *statisticsConfigRead) TransformAndValidate() (ret StatisticsConfig, err []error) {
	// default values
	ret.enabled = false
	ret.historyResolution = time.Second
	ret.historyMaxAge = 10 * time.Minute

	if c == nil {
		return
	}

	if c.Enabled != nil && *c.Enabled {
		ret.enabled = true
	}

	if len(c.HistoryResolution) < 1 {
		// use default 1s
	} else if historyResolution, e := time.ParseDuration(c.HistoryResolution); e != nil {
		err = append(err, fmt.Errorf("Statistics->HistoryResolution='%s' parse error: %s",
			c.HistoryResolution, e,
		))
	} else if historyResolution <= 0 {
		err = append(err, fmt.Errorf("Statistics->HistoryResolution='%s' must be >0",
			c.HistoryResolution,
		))
	} else {
		ret.historyResolution = historyResolution
	}

	if len(c.HistoryMaxAge) < 1 {
		// use default 10min
	} else if historyMaxAge, e := time.ParseDuration(c.HistoryMaxAge); e != nil {
		err = append(err, fmt.Errorf("Statistics->HistoryMaxAge='%s' parse error: %s",
			c.HistoryMaxAge, e,
		))
	} else if historyMaxAge <= 0 {
		err = append(err, fmt.Errorf("Statistics->HistoryMaxAge='%s' must be >0",
			c.HistoryMaxAge,
		))
	} else {
		ret.historyMaxAge = historyMaxAge
	}

	return
}

func (c mqttClientConfigReadMap) getOrderedKeys() (ret []string) {
	ret = make([]string, len(c))
	i := 0
	for k := range c {
		ret[i] = k
		i++
	}
	sort.Strings(ret)
	return
}

func (c mqttClientConfigReadMap) TransformAndValidate() (ret []*MqttClientConfig, err []error) {
	if len(c) < 1 {
		return ret, []error{fmt.Errorf("MqttClients section must no be empty")}
	}

	ret = make([]*MqttClientConfig, len(c))
	j := 0
	for _, name := range c.getOrderedKeys() {
		r, e := c[name].TransformAndValidate(name)
		ret[j] = &r
		err = append(err, e...)
		j++
	}
	return
}

func (c mqttClientConfigRead) TransformAndValidate(name string) (ret MqttClientConfig, err []error) {
	ret = MqttClientConfig{
		name:        name,
		broker:      c.Broker,
		user:        c.User,
		password:    c.Password,
		clientId:    c.ClientId,
		topicPrefix: c.TopicPrefix,
	}

	if !nameMatcher.MatchString(ret.name) {
		err = append(err, fmt.Errorf("MqttClientConfig->Name='%s' does not match %s", ret.name, NameRegexp))
	}

	if len(ret.broker) < 1 {
		err = append(err, fmt.Errorf("MqttClientConfig->%s->Broker must not be empty", name))
	}
	if len(ret.clientId) < 1 {
		ret.clientId = "go-mqtt-to-influx"
	}
	if c.Qos == nil {
		ret.qos = 0
	} else if *c.Qos == 0 || *c.Qos == 1 || *c.Qos == 2 {
		ret.qos = *c.Qos
	} else {
		err = append(err, fmt.Errorf("MqttClientConfig->%s->Qos=%d but must be 0, 1 or 2", name, *c.Qos))
	}

	if c.AvailabilityTopic == nil {
		// use default
		ret.availabilityTopic = "%Prefix%tele/%clientId%/LWT"
	} else {
		ret.availabilityTopic = *c.AvailabilityTopic
	}

	if c.LogMessages != nil && *c.LogMessages {
		ret.logMessages = true
	}

	return
}

func (c influxClientConfigReadMap) getOrderedKeys() (ret []string) {
	ret = make([]string, len(c))
	i := 0
	for k := range c {
		ret[i] = k
		i++
	}
	sort.Strings(ret)
	return
}

func (c influxClientConfigReadMap) TransformAndValidate() (ret []*InfluxClientConfig, err []error) {
	if len(c) < 1 {
		return ret, []error{fmt.Errorf("InfluxClients section must no be empty")}
	}

	ret = make([]*InfluxClientConfig, len(c))
	j := 0
	for _, name := range c.getOrderedKeys() {
		r, e := c[name].TransformAndValidate(name)
		ret[j] = &r
		err = append(err, e...)
		j++
	}
	return
}

func (c influxClientConfigRead) TransformAndValidate(name string) (ret InfluxClientConfig, err []error) {
	ret = InfluxClientConfig{
		name:     name,
		address:  c.Address,
		user:     c.User,
		password: c.Password,
		database: c.Database,
	}

	if !nameMatcher.MatchString(ret.name) {
		err = append(err, fmt.Errorf("InfluxClientConfig->Name='%s' does not match %s", ret.name, NameRegexp))
	}

	if len(ret.address) < 1 {
		err = append(err, fmt.Errorf("InfluxClientConfig->%s->Address must not be empty", name))
	}

	if len(ret.database) < 1 {
		ret.database = "go-mqtt-to-influx"
	}

	if len(c.WriteInterval) < 1 {
		// use default 0
		ret.writeInterval = 200 * time.Millisecond
	} else if writeInterval, e := time.ParseDuration(c.WriteInterval); e != nil {
		err = append(err, fmt.Errorf("InfluxClientConfig->%s->WriteInterval='%s' parse error: %s",
			name, c.WriteInterval, e,
		))
	} else if writeInterval < 0 {
		err = append(err, fmt.Errorf("InfluxClientConfig->%s->WriteInterval='%s' must be positive",
			name, c.WriteInterval,
		))
	} else {
		ret.writeInterval = writeInterval
	}

	if len(c.TimePrecision) < 1 {
		// use default 1s
		ret.timePrecision = time.Second
	} else if timePrecision, e := time.ParseDuration(c.TimePrecision); e != nil {
		err = append(err, fmt.Errorf("InfluxClientConfig->%s->TimePrecision='%s' parse error: %s",
			name, c.TimePrecision, e,
		))
	} else if timePrecision < 0 {
		err = append(err, fmt.Errorf("InfluxClientConfig->%s->TimePrecision='%s' must be positive",
			name, c.TimePrecision,
		))
	} else {
		ret.timePrecision = timePrecision
	}

	if c.LogLineProtocol != nil && *c.LogLineProtocol {
		ret.logLineProtocol = true
	}

	return
}

func (c converterConfigReadMap) getOrderedKeys() (ret []string) {
	ret = make([]string, len(c))
	i := 0
	for k := range c {
		ret[i] = k
		i++
	}
	sort.Strings(ret)
	return
}

func (c converterConfigReadMap) TransformAndValidate(
	mqttClients []*MqttClientConfig,
	influxClients []*InfluxClientConfig,
) (ret []*ConverterConfig, err []error) {
	if len(c) < 1 {
		return ret, []error{fmt.Errorf("Converters section must no be empty.")}
	}

	ret = make([]*ConverterConfig, len(c))
	j := 0
	for _, name := range c.getOrderedKeys() {
		r, e := c[name].TransformAndValidate(name, mqttClients, influxClients)
		ret[j] = &r
		err = append(err, e...)
		j++
	}
	return
}

var implementationsAndDefaultMeasurement = map[string]string{
	"go-ve-sensor":   "floatValue",
	"lwt":            "boolValue",
	"tasmota-state":  "boolValue",
	"tasmota-sensor": "floatValue",
}

func (c converterConfigRead) TransformAndValidate(
	name string,
	mqttClients []*MqttClientConfig,
	influxClients []*InfluxClientConfig,
) (ret ConverterConfig, err []error) {
	ret = ConverterConfig{
		name:              name,
		implementation:    c.Implementation,
		targetMeasurement: c.TargetMeasurement,
		mqttTopics:        c.MqttTopics,
		mqttClients:       c.MqttClients,
		influxClients:     c.InfluxClients,
	}

	if !nameMatcher.MatchString(ret.name) {
		err = append(err, fmt.Errorf("Converters->Name='%s' does not match %s", ret.name, NameRegexp))
	}

	if def, ok := implementationsAndDefaultMeasurement[ret.implementation]; !ok {
		err = append(err, fmt.Errorf("Converters->%s->Implementation='%s' is unkown", name, ret.implementation))
	} else if len(ret.targetMeasurement) < 1 {
		ret.targetMeasurement = def
	}

	// validate that all listed mqttClients exist
	for _, clientName := range ret.mqttClients {
		found := false
		for _, client := range mqttClients {
			if clientName == client.name {
				found = true
				break
			}
		}

		if !found {
			err = append(err, fmt.Errorf("Converters->%s->MqttClient='%s' is not defined", name, clientName))
		}
	}

	// validate that all listed influxClients exist
	for _, clientName := range ret.influxClients {
		found := false
		for _, client := range influxClients {
			if clientName == client.name {
				found = true
				break
			}
		}

		if !found {
			err = append(err, fmt.Errorf("Converters->%s->InfluxClient='%s' is not defined", name, clientName))
		}
	}

	if len(ret.mqttTopics) < 1 {
		err = append(err, fmt.Errorf("Converters->%s->MqttTopics must not be empty", name))
	}

	if c.LogHandleOnce != nil && *c.LogHandleOnce {
		ret.logHandleOnce = true
	}

	return
}
