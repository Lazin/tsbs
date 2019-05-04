package akumuli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/timescale/tsbs/cmd/tsbs_generate_queries/uses/devops"
	"github.com/timescale/tsbs/query"
)

// Devops produces Influx-specific queries for all the devops query types.
type Devops struct {
	*devops.Core
}

// NewDevops makes an Devops object ready to generate Queries.
func NewDevops(start, end time.Time, scale int) *Devops {
	return &Devops{devops.NewCore(start, end, scale)}
}

// GenerateEmptyQuery returns an empty query.HTTP
func (d *Devops) GenerateEmptyQuery() query.Query {
	return query.NewHTTP()
}

type tsdbQueryRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

type tsdbGroupAggStmt struct {
	Name []string `json:"metric"`
	Func []string `json:"func"`
	Step string   `json:"step"`
}

type tsdbGroupAggregateQuery struct {
	GroupAggregate tsdbGroupAggStmt    `json:"group-aggregate"`
	TimeRange      tsdbQueryRange      `json:"range"`
	Where          map[string][]string `json:"where"`
	Output         map[string]string   `json:"output"`
	OrderBy        string              `json:"order-by"`
}

type tsdbSelectQuery struct {
	Select    string                       `json:"select"`
	TimeRange tsdbQueryRange               `json:"range"`
	Where     map[string][]string          `json:"where"`
	Output    map[string]string            `json:"output"`
	Filter    map[string]map[string]string `json:"filter"`
}

type tsdbAggregateAllQuery struct {
	Metrics map[string]string `json:"aggregate"`
	Output  map[string]string `json:"output"`
}

// GroupByTime selects the MAX for a single metric under 'cpu',
// per minute for nhosts hosts,
// e.g. in pseudo-SQL:
//
// SELECT minute, max(metric)
// FROM cpu
// WHERE
// 		(hostname = '$HOSTNAME_1' OR ... OR hostname = '$HOSTNAME_N')
// 	AND time >= '$HOUR_START'
// 	AND time < '$HOUR_END'
// GROUP BY minute
// ORDER BY minute ASC
//
// Resultsets:
// single-groupby-1-1-12
// single-groupby-1-1-1
// single-groupby-1-8-1
// single-groupby-5-1-12
// single-groupby-5-1-1
// single-groupby-5-8-1
func (d *Devops) GroupByTime(qi query.Query, nhosts, numMetrics int, timeRange time.Duration) {
	interval := d.Interval.RandWindow(timeRange)
	hostnames := d.GetRandomHosts(nhosts)
	startTimestamp := interval.StartUnixNano()
	endTimestamp := interval.EndUnixNano()

	var query tsdbGroupAggregateQuery
	query.GroupAggregate.Func = append(query.GroupAggregate.Func, "max")
	query.GroupAggregate.Step = "1m"
	for _, name := range devops.GetCPUMetricsSlice(numMetrics) {
		query.GroupAggregate.Name = append(query.GroupAggregate.Name, "cpu."+name)
	}

	query.Where = make(map[string][]string)
	query.Where["hostname"] = hostnames
	query.TimeRange.From = startTimestamp
	query.TimeRange.To = endTimestamp
	query.Output = make(map[string]string)
	query.Output["format"] = "csv"
	query.OrderBy = "time"

	bodyWriter := new(bytes.Buffer)
	body, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}
	bodyWriter.Write(body)

	humanLabel := fmt.Sprintf("Akumuli max cpu, rand %4d hosts, rand %s by 1m", nhosts, timeRange)
	humanDesc := fmt.Sprintf("%s: %s", humanLabel, interval.StartString())
	d.fillInQuery(qi, humanLabel, humanDesc, string(bodyWriter.Bytes()), interval.StartUnixNano(), interval.EndUnixNano())
}

// HighCPUForHosts populates a query that gets CPU metrics when the CPU has high
// usage between a time period for a number of hosts (if 0, it will search all hosts),
// e.g. in pseudo-SQL:
//
// SELECT * FROM cpu
// WHERE usage_user > 90.0
// AND time >= '$TIME_START' AND time < '$TIME_END'
// AND (hostname = '$HOST' OR hostname = '$HOST2'...)
//
// Resultsets:
// high-cpu-1
// high-cpu-all
func (d *Devops) HighCPUForHosts(qi query.Query, nHosts int) {
	interval := d.Interval.RandWindow(devops.HighCPUDuration)
	var hostnames []string

	if nHosts > 0 {
		hostnames = d.GetRandomHosts(nHosts)
	}
	startTimestamp := interval.StartUnixNano()
	endTimestamp := interval.EndUnixNano()
	var query tsdbSelectQuery
	query.Select = "cpu.usage_user"
	query.Where = make(map[string][]string)
	if nHosts > 0 {
		query.Where["hostname"] = hostnames
	}
	query.TimeRange.From = startTimestamp
	query.TimeRange.To = endTimestamp
	query.Output = make(map[string]string)
	query.Output["format"] = "csv"
	query.Filter = make(map[string]map[string]string)
	query.Filter["cpu"] = make(map[string]string)
	query.Filter["cpu"]["gt"] = "90.0"

	bodyWriter := new(bytes.Buffer)
	body, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}
	bodyWriter.Write(body)

	humanLabel := devops.GetHighCPULabel("Akumuli", nHosts)
	humanDesc := fmt.Sprintf("%s: %s", humanLabel, interval)
	d.fillInQuery(qi, humanLabel, humanDesc, string(bodyWriter.Bytes()), interval.StartUnixNano(), interval.EndUnixNano())
}

// MaxAllCPU selects the MAX of all metrics under 'cpu' per hour for nhosts hosts,
// e.g. in pseudo-SQL:
//
// SELECT MAX(metric1), ..., MAX(metricN)
// FROM cpu
// WHERE
// 		(hostname = '$HOSTNAME_1' OR ... OR hostname = '$HOSTNAME_N')
// 		AND time >= '$HOUR_START'
// 		AND time < '$HOUR_END'
// GROUP BY hour
// ORDER BY hour
//
// Resultsets:
// cpu-max-all-1
// cpu-max-all-8
func (d *Devops) MaxAllCPU(qi query.Query, nHosts int) {
	interval := d.Interval.RandWindow(devops.MaxAllDuration)
	hostnames := d.GetRandomHosts(nHosts)
	startTimestamp := interval.StartUnixNano()
	endTimestamp := interval.EndUnixNano()

	var query tsdbGroupAggregateQuery
	query.GroupAggregate.Func = append(query.GroupAggregate.Func, "max")
	query.GroupAggregate.Step = "1h"
	for _, name := range devops.GetAllCPUMetrics() {
		query.GroupAggregate.Name = append(query.GroupAggregate.Name, "cpu."+name)
	}

	query.Where = make(map[string][]string)
	query.Where["hostname"] = hostnames
	query.TimeRange.From = startTimestamp
	query.TimeRange.To = endTimestamp
	query.Output = make(map[string]string)
	query.Output["format"] = "csv"
	query.OrderBy = "time"

	bodyWriter := new(bytes.Buffer)
	body, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}
	bodyWriter.Write(body)

	humanLabel := devops.GetMaxAllLabel("Akumuli", nHosts)
	humanDesc := fmt.Sprintf("%s: %s", humanLabel, interval.StartString())
	d.fillInQuery(qi, humanLabel, humanDesc, string(bodyWriter.Bytes()), interval.StartUnixNano(), interval.EndUnixNano())
}

// LastPointPerHost finds the last row for every host in the dataset
func (d *Devops) LastPointPerHost(qi query.Query) {

	var query tsdbAggregateAllQuery
	query.Metrics = make(map[string]string)
	for _, name := range devops.GetAllCPUMetrics() {
		query.Metrics["cpu."+name] = "last"
	}
	query.Output = make(map[string]string)
	query.Output["format"] = "csv"

	bodyWriter := new(bytes.Buffer)
	body, err := json.Marshal(query)
	if err != nil {
		panic(err)
	}
	bodyWriter.Write(body)

	humanLabel := "Akumuli last row per host"
	humanDesc := humanLabel + ": cpu"
	d.fillInQuery(qi, humanLabel, humanDesc, string(bodyWriter.Bytes()), 0, 0)
}

func (d *Devops) fillInQuery(qi query.Query, humanLabel, humanDesc, body string, begin, end int64) {
	q := qi.(*query.HTTP)
	q.HumanLabel = []byte(humanLabel)
	q.HumanDescription = []byte(humanDesc)
	q.Method = []byte("POST")
	q.Path = []byte("/api/query")
	q.Body = []byte(body)
	q.StartTimestamp = begin
	q.EndTimestamp = end
}
