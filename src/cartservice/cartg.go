package main

import (
	"fmt"
	"net"
	"os"
	"time"
	//"math"
	"encoding/json"
	//"strconv"
	"io"

	"github.com/redis/go-redis/v9"

	"cloud.google.com/go/profiler"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	//"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/cartservice/genproto"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

var rClient *redis.Client


const (
	defaultPort = "7070"
	redisAddr = "redis-cach:6379"
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

	rClient = redis.NewClient(&redis.Options{
		Addr:	  redisAddr,
		Password: "", // no password set
		DB:		  0,  // use default DB
	})

	log.Infof("Connected to Redis successfuly")

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



	svc := &cartServer{}
	pb.RegisterCartServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)
	log.Infof("Cart Service listening on port %s", port)

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to server: %v", err)
	}

}

// server controls RPC service responses.
type cartServer struct{}

// Check is for health checking.
func (s *cartServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *cartServer) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
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

func (s *cartServer) AddItem(ctx context.Context, in *pb.AddItemRequest) (*pb.Empty, error) {

//	fmt.Println("Got Add Item request");

//	log.Infof("Session: %s", in.UserId)

	redisCtx := context.Background()

	userCart, err := rClient.Get(redisCtx, in.UserId).Result()

	var newCart pb.Cart
	
	if err == redis.Nil  {

		newItem := &pb.CartItem{
			ProductId: in.Item.ProductId,
			Quantity: in.Item.Quantity,
		}

		var allItems []*pb.CartItem

		allItems = append(allItems, newItem)

		newCart = pb.Cart{UserId: in.UserId, Items: allItems}

	} else {

		unerr := json.Unmarshal([]byte(userCart), &newCart)
		if unerr != nil {
			log.Info("Redis retutned error")
		} else {

			var exists bool

			for _, item := range newCart.Items {
				if item.ProductId == in.Item.ProductId {
					exists = true
					item.Quantity = item.Quantity + in.Item.Quantity
				}
			}

			if exists != true {
				anotherItem := &pb.CartItem{ProductId: in.Item.ProductId, Quantity: in.Item.Quantity}
				newCart.Items = append(newCart.Items, anotherItem)
			}

		}

	}

	b, _ := json.Marshal(newCart)

	seterr := rClient.Set(redisCtx, in.UserId, string(b), 0).Err()
	if seterr != nil {
		log.Info("Failed to redis")
	}

	//fmt.Println(newCart)

	return &pb.Empty{}, nil
}

func (s *cartServer) GetCart(ctx context.Context, in *pb.GetCartRequest) (*pb.Cart, error) {

//	fmt.Println("Got Get cart request");

	redisCtx := context.Background()

	clientCart, err := rClient.Get(redisCtx, in.UserId).Result()
	if err != nil {
		//log.Info("Something bad happened with Redis")
		return &pb.Cart{}, nil
	}

	var cleanCart pb.Cart

	reserr := json.Unmarshal([]byte(clientCart),&cleanCart)
	if reserr != nil {
		return &pb.Cart{}, nil
	}

	return &cleanCart, nil
}

func (s *cartServer) EmptyCart(ctx context.Context, in *pb.EmptyCartRequest) (*pb.Empty, error) {

//	fmt.Println("Got empty cart request");

	redisCtx := context.Background()

	_, err := rClient.Do(redisCtx, "del", in.UserId).Result()

	if err != nil {
		return &pb.Empty{}, nil
	}

	return &pb.Empty{}, nil
}
