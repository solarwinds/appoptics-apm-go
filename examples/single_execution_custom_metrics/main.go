package main

import (
	"context"
	"github.com/appoptics/appoptics-apm-go/v1/ao"
	"log"
	"time"
)

func main() {
	ao.WaitForReady(context.Background())

	sleepSec := 65.0

	err := ao.SummaryMetric("myTestSummaryMetric", sleepSec, ao.MetricOptions{Count: 1})
	log.Printf("Reported summary metrics, err=%v", err)
	err = ao.IncrementMetric("myTestIncrementMetric", ao.MetricOptions{Count: 1})
	log.Printf("Reported increment metrics, err=%v", err)

	time.Sleep(time.Second * time.Duration(sleepSec))
	log.Println("Done!")
}
