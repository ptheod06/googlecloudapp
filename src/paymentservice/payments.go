package main

import (
	"fmt"
	"net"
	"os"
	"time"
	//"math"
	"strconv"
	"strings"
	"io"

	"github.com/google/uuid"
	"github.com/durango/go-credit-card"


	"cloud.google.com/go/profiler"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	//"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/paymentservice/genproto"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)


const (
	defaultPort = "50060"
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
	log.SetOutput(io.Discard)
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



	svc := &paymentServer{}
	pb.RegisterPaymentServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)
	log.Infof("Payment Service listening on port %s", port)

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to server: %v", err)
	}

}

// server controls RPC service responses.
type paymentServer struct{}

// Check is for health checking.
func (s *paymentServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *paymentServer) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
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

func (s *paymentServer) Charge(ctx context.Context, in *pb.ChargeRequest) (*pb.ChargeResponse, error) {

//	fmt.Println("Got Charge request");

	cardYear := strconv.Itoa(int(in.CreditCard.CreditCardExpirationYear))
	cardMonth := strconv.Itoa(int(in.CreditCard.CreditCardExpirationMonth))

	cardNo := strings.ReplaceAll(in.CreditCard.CreditCardNumber, "-", "")

	card := creditcard.Card{Number: cardNo, Cvv: "123", Month: cardMonth, Year: cardYear}

	err := card.Method()

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to validate credit card number")
	}


	if card.Company.Short != "visa" && card.Company.Short != "mastercard" {
		return nil, status.Errorf(codes.Internal, "Only accepting visa and mastercard")
	}

	expErr := card.ValidateExpiration()

	if expErr != nil {
		return nil, status.Errorf(codes.Internal, "Card is expired")
	}

	val, _ := uuid.NewRandom()

	cleaned := val.String()

	return &pb.ChargeResponse{TransactionId: cleaned}, nil


}
