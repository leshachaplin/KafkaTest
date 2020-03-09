package cmd

import (
	"context"
	"fmt"
	"github.com/labstack/echo"
	"github.com/leshachaplin/KafkaTest/config"
	"github.com/leshachaplin/KafkaTest/dispatcher"
	"github.com/leshachaplin/KafkaTest/operations"
	Kafka "github.com/leshachaplin/communicationUtils/kafka"
	Websocket "github.com/leshachaplin/communicationUtils/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"
)

var (
	isWatcher  bool
	serverPort int
	clientPort int
	mu         sync.Mutex
)

type Message struct {
	Time string `json:"time"`
}

var rootCmd = &cobra.Command{
	Use:   "",
	Short: "",
	Long:  `.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(isWatcher)
		s := make(chan os.Signal)
		signal.Notify(s, os.Interrupt)
		done, cnsl := context.WithCancel(context.Background())
		d := &dispatcher.Dispatcher{
			Operation: make(chan dispatcher.Operation),
		}
		cfg := config.NewConfig()
		e := echo.New()

		_, err := Websocket.NewServer(OnMsg(d), done, e)
		if err != nil {
			log.Errorf("server not connected")
		}

		go func(e *echo.Echo) {
			e.Start(fmt.Sprintf(":%d", serverPort))
		}(e)

		time.Sleep(time.Second * 10)
		ws, err := Websocket.NewClient(cfg.Origin, cfg.Url)
		if err != nil {
			log.Errorf("websocket not dial", err)
		}
		defer ws.Close()

		k2, err := Kafka.New("time200", cfg.KafkaUrl, strconv.FormatBool(!isWatcher))

		d.OnOperation = func(operation dispatcher.Operation) {
			switch operation.(type) {
			case operations.ReadRole:
				{
					fmt.Println("READ ROLE")
					go func() {
						fmt.Println("read role work")
						role := operation.(operations.ReadRole)
						if role.Role == "read" {
							fmt.Println("send read operation")
							d.Operation <- operations.ReadOperationMessage{}
							if d.CancelWrite != nil {
								d.CancelWrite <- operations.CancelWrite{}
								fmt.Println("send cancel write")
							} else {
								d.CancelWrite = make(chan dispatcher.Operation)
								//d.CancelWrite <- CancelWrite{}
								//fmt.Println("SEND CANCEL WRITE")
							}
						} else {
							fmt.Println("send write operation")
							d.Operation <- operations.WriteOperationMessage{}

							if d.CancelRead != nil {
								d.CancelRead <- operations.CancelRead{}
								fmt.Println("send cancel read")
							} else {
								d.CancelRead = make(chan dispatcher.Operation)
								//d.CancelRead <- CancelRead{}
								//fmt.Println("SEND CANCEL READ")
							}
						}
					}()
				}
			case operations.ReadOperationMessage:
				{
					fmt.Println("read Message")
					go func() {
						for {
							select {
							case <-d.CancelRead:
								{
									fmt.Println("cancel read")
									return
								}
							default:
								{
									mes, err := k2.ReadMessage()
									if err != nil {
										log.Errorf("message not read", err)
									}
									fmt.Println(string(mes))

									time.Sleep(time.Second)
								}
							}
						}
					}()
				}
			case operations.WriteOperationMessage:
				{
					fmt.Println("write Message")
					go func() {
						for {
							select {
							case <-d.CancelWrite:
								{
									fmt.Println("Cancel write")
									return
								}
							default:
								{
									if err != nil {
										log.Errorf("message not Send", err)
									}
									err = k2.WriteMessage([]byte(time.Now().String()))
									if err != nil {
										log.Errorf("message not Send", err)
									}
									fmt.Println("SEND MESSAGE FFROM K2")
									time.Sleep(time.Second * 1)
								}
							}
						}
					}()
				}
			}
		}

		dispatcher.Do(d, done)

		if isWatcher {
			d.Operation <- operations.ReadOperationMessage{}
		} else {
			d.Operation <- operations.WriteOperationMessage{}
		}

		<-s
		close(s)
		cnsl()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&isWatcher, "watcher", false, "User role for app")
	rootCmd.PersistentFlags().IntVar(&serverPort, "serverPort", 8888, "User role for app")
	rootCmd.PersistentFlags().IntVar(&clientPort, "clientPort", 7777, "User role for app")
}

func Execute() error {
	return rootCmd.Execute()
}

func OnMsg(dis *dispatcher.Dispatcher) func(msg string) {
	return func(msg string) {
		dis.Operation <- &operations.ReadRole{Role: msg}
	}
}
