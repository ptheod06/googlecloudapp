// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"net"
	"os"
	"time"
	"math"
	"encoding/json"
	"strconv"

	"cloud.google.com/go/profiler"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	//"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/currencyservice/genproto"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)


type Money_Temp struct {
	units float64
	nanos float64
}

const (
	defaultPort = "7000"
)

var (
	converted_money Money_Temp
)

var log *logrus.Logger


func init() {
	log = logrus.New()
	log.Level = logrus.DebugLevel
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
}

func main() {
	if os.Getenv("ENABLE_TRACING") == "1" {
		log.Info("Tracing enabled, but temporarily unavailable")
		log.Info("See https://github.com/GoogleCloudPlatform/microservices-demo/issues/422 for more info.")
		go initTracing()
	} else {
		log.Info("Tracing disabled.")
	}

	if os.Getenv("DISABLE_PROFILER") == "" {
		log.Info("Profiling enabled.")
		go initProfiling("shippingservice", "1.0.0")
	} else {
		log.Info("Profiling disabled.")
	}

	port := defaultPort

	if value, ok := os.LookupEnv("PORT"); ok {
		port = value
	}

	port = fmt.Sprintf(":%s", port)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var srv *grpc.Server

	if os.Getenv("DISABLE_STATS") == "" {
		log.Info("Stats enabled, but temporarily unavailable")
		srv = grpc.NewServer()
	} else {
		log.Info("Stats disabled.")
		srv = grpc.NewServer()
	}


	svc := &currencyServer{}
	pb.RegisterCurrencyServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)
	log.Infof("Currency Service listening on port %s", port)

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to server: %v", err)
	}

}

// server controls RPC service responses.
type currencyServer struct{}

// Check is for health checking.
func (s *currencyServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *currencyServer) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
}



//Helper function to read currencies from file
func getCurrencyData() (map[string]float64){



	da, err := os.ReadFile("curr.json")

	data := make(map[string]string)

	currencies := make(map[string]float64)

	json.Unmarshal(da, &data)

	if err != nil {
		fmt.Println("Could not read currencies file")
		return nil
	} else {
		for key, val := range data {
			converted, _ := strconv.ParseFloat(val, 64)

			currencies[key] = converted

		}

	}

	return currencies
}

// Convert between currencies
func convert_helper(units int64, nanos int32, currency_code string, to_code string) {

//	log.Info("Units %v", units)

	var currency_list map[string]float64

	currency_list = getCurrencyData()

	var nAmount = Money_Temp{(float64(units)/currency_list[currency_code]), (float64(nanos)/currency_list[currency_code])};

	var ineuros = carry(nAmount)

	ineuros.nanos = math.Round(ineuros.nanos)

	var nResult = Money_Temp{(ineuros.units * currency_list[to_code]), (ineuros.nanos * currency_list[to_code])}

//	log.Info("Before: %v", nResult.units)

	converted_money = carry(nResult)

//	log.Info("Converted Units", converted_money.units)
//	log.Info("Converted Nanso", converted_money.nanos)

//	log.Info("Converted: %v", converted_money.units)

	converted_money.units = math.Floor(converted_money.units)
	converted_money.nanos = math.Floor(converted_money.nanos)

//	log.Info("converted unis", converted_money.units)
//	log.Info("converted nanos", converted_money.nanos)

}

//Handle carry operations
func carry(amount Money_Temp) (Money_Temp) {

	var fractionSize float64
	fractionSize = math.Pow(10, 9)

	amount.nanos += math.Mod(amount.units, 1) * fractionSize

	amount.units = math.Floor(amount.units) + math.Floor(amount.nanos / fractionSize)

	amount.nanos = math.Mod(amount.nanos, fractionSize)

//	log.Info("Units: %d", amount.units)
//	log.Info("Nanos: %d", amount.nanos)

	return amount

}




// Get all supported currencies produces a shipping quote (cost) in USD.
func (s *currencyServer) GetSupportedCurrencies(ctx context.Context, in *pb.Empty) (*pb.GetSupportedCurrenciesResponse, error) {

//	log.Info("Got request on all currencies")

	var curr_codes []string

	var all_currencies map[string]float64

	all_currencies = getCurrencyData()

	for key, _ := range all_currencies {
		curr_codes = append(curr_codes, key)
	}
	
	return &pb.GetSupportedCurrenciesResponse{CurrencyCodes: curr_codes}, nil
	

}

func (s *currencyServer) Convert(ctx context.Context, in *pb.CurrencyConversionRequest) (*pb.Money, error) {

//	log.Info("Got request on conversion")
//	log.Info("To Code %v", in.ToCode)
//	log.Info("Price %v", in.From.Units)

	convert_helper(in.From.Units, in.From.Nanos, in.From.CurrencyCode, in.ToCode);

//	log.Info("Conversion: ", converted_money.units)

	return &pb.Money{
		CurrencyCode: in.ToCode,
		Units: int64(converted_money.units),
		Nanos: int32(converted_money.nanos),
		}, nil

}


func initStats() {
	//TODO(arbrown) Implement OpenTelemetry stats
}

func initTracing() {
	// TODO(arbrown) Implement OpenTelemetry tracing
}

func initProfiling(service, version string) {
	// TODO(ahmetb) this method is duplicated in other microservices using Go
	// since they are not sharing packages.
	for i := 1; i <= 3; i++ {
		if err := profiler.Start(profiler.Config{
			Service:        service,
			ServiceVersion: version,
			// ProjectID must be set if not running on GCP.
			// ProjectID: "my-project",
		}); err != nil {
			log.Warnf("failed to start profiler: %+v", err)
		} else {
			log.Info("started Stackdriver profiler")
			return
		}
		d := time.Second * 10 * time.Duration(i)
		log.Infof("sleeping %v to retry initializing Stackdriver profiler", d)
		time.Sleep(d)
	}
	log.Warn("could not initialize Stackdriver profiler after retrying, giving up")
}
