package main

// #cgo CFLAGS: -g -Wall
// #cgo LDFLAGS: -lwiringPi
// #include <stdio.h>
// #include <stdlib.h>
// #include <string.h>
// #include <time.h>
// #include <unistd.h>
// #include <wiringPi.h>
// #include "main.h"
import "C"

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"unsafe"

	pb "github.com/Darkhunter9/lazIR_tag/src/proto"
	_ "github.com/Jille/grpc-multi-resolver"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/health"
)

var (
	config = flag.String("config", "", "config file path")
)

func client(dns string) (*pb.ScoreClient, *grpc.ClientConn, error) {
	serviceConfig := `{"healthCheckConfig": {"serviceName": "Score"}, "loadBalancingConfig": [ { "round_robin": {} } ]}`
	retryOpts := []grpc_retry.CallOption{
		grpc_retry.WithBackoff(grpc_retry.BackoffExponential(100 * time.Millisecond)),
		grpc_retry.WithMax(5),
	}

	conn, err := grpc.Dial(dns,
		grpc.WithDefaultServiceConfig(serviceConfig), grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(retryOpts...)))

	if err != nil {
		log.Fatalf("dialing failed: %v", err)
	}

	c := pb.NewScoreClient(conn)

	return &c, conn, nil
}

func addRecord(c *pb.ScoreClient, shooter string, victim string) error {
	cli := *c
	_, err2 := cli.AddRecord(
		context.Background(),
		&pb.AddRecordRequest{Shooter: shooter, Victim: victim},
	)

	if err2 != nil {
		return fmt.Errorf("addRecord: %v", err2)
	}

	return nil
}

func getRecord(c *pb.ScoreClient) (map[string]int32, error) {
	cli := *c
	res, err2 := cli.GetRecord(
		context.Background(),
		&pb.GetRecordRequest{},
	)

	if err2 != nil {
		return nil, fmt.Errorf("getRecord: %v", err2)
	}

	return res.Score, nil
}

func main() {
	flag.Parse()

	if *config == "" {
		log.Fatalf("flag --config is required")
	}
	conf := getConf(*config)

	user := conf.RaftId
	dns := "multi:///"
	for i, ip := range conf.AddressPool {
		if i == 0 {
			dns = dns + ip
		} else {
			dns = dns + "," + ip
		}
	}
	log.Printf("RaftId: %s", user)
	log.Printf("Address pool: %s", dns)

	c, conn, errConn := client(dns)
	defer conn.Close()
	if errConn != nil {
		log.Fatal("error connecting, exiting")
	}

	ptr := C.malloc(C.sizeof_char * 30) // allocate 30 bytes
	defer C.free(unsafe.Pointer(ptr))

	for {
		str := string(C.GoBytes(unsafe.Pointer(C.mainC()), 3))
		fmt.Println("received: ", str)

		errAddRecord := addRecord(c, str, user)
		if errAddRecord != nil {
			log.Fatal("error calling addRecord")
		}

		// check if user is dead
		score, errGetRecord := getRecord(c)
		if errGetRecord != nil {
			log.Fatal("error calling GetRecord")
		}
		fmt.Println("user's current score is: ", score[user])
		if score[user] == 0 {
			fmt.Println("user score reached 0, exiting...")
			os.Exit(0)
		}
		time.Sleep(time.Duration(500) * time.Millisecond)
	}
}
