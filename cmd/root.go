package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo"
	"github.com/leshachaplin/Dispatcher/Kafka"
	"github.com/leshachaplin/Dispatcher/Websocket"
	"github.com/leshachaplin/Dispatcher/dispatcher"
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

type ReadRole struct {
	Role string
}

type WriteRole struct {
}

type ReadOperationMessage struct {
}

type WriteOperationMessage struct {
}

type WriteOperationMessageFromSecondKafka struct {
}

type CancelWriteFromSecondKafka struct {
}

type CancelWrite struct {
}

type CancelRead struct {
}

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
			Operation: make(chan dispatcher.Operation, 3),
		}

		w, err := Websocket.New(OB(d), done, serverPort)
		if err != nil {
			log.Errorf("websocket not dial", err)
		}

		go func(e *echo.Echo) {
			e.Start(fmt.Sprintf(":%d", serverPort))
		}(w.Echo)

		time.Sleep(time.Second * 10)
		ws, err := w.NewWebsocketConnection()
		if err != nil {
			log.Errorf("websocket not dial", err)
		}
		defer ws.Close()

		k, err := Kafka.New("time115", 9092, strconv.FormatBool(isWatcher))
		defer k.Close()

		k2, err := Kafka.New("time114", 9092, strconv.FormatBool(!isWatcher))

		d.OnOperation = func(operation dispatcher.Operation) {
			switch operation.(type) {
			case ReadRole:
				{
					fmt.Println("READ ROLE")
					go func() {
						fmt.Println("read role work")
						role := operation.(ReadRole)
						if role.Role == "read" {
							fmt.Println("send read operation")
							d.Operation <- ReadOperationMessage{}
							if d.CancelWrite != nil {
								d.CancelWrite <- CancelWriteFromSecondKafka{}
								fmt.Println("send cancel write")
							} else {
								d.CancelWrite = make(chan dispatcher.Operation)
								//d.CancelWrite <- CancelWrite{}
								//fmt.Println("SEND CANCEL WRITE")
							}
						} else {
							fmt.Println("send write operation")
							d.Operation <- WriteOperationMessage{}
							d.Operation <- WriteOperationMessageFromSecondKafka{}

							if d.CancelRead != nil {
								d.CancelRead <- CancelRead{}
								fmt.Println("send cancel read")
							} else {
								d.CancelRead = make(chan dispatcher.Operation)
								//d.CancelRead <- CancelRead{}
								//fmt.Println("SEND CANCEL READ")
							}
						}
					}()
				}
			case WriteRole:
				{
					fmt.Println("WRITE ROLE")
					go func() {
						fmt.Println(fmt.Sprintf("i'am watcher %v change role %d", isWatcher, clientPort))
						mes := "read"
						if isWatcher {
							mes = "write"
							isWatcher = false
						} else {
							isWatcher = true
						}
						if _, err := ws.Write([]byte(mes)); err != nil {
							log.Errorf("message not send", err)
						}

						if !isWatcher { //????????????????????????????????????????????????????????????????
							if d.CancelRead != nil {
								d.CancelRead <- CancelRead{}
								fmt.Println("send cancel READ")
							} else {
								d.CancelRead = make(chan dispatcher.Operation)
								d.CancelRead <- CancelRead{}
								fmt.Println("CANCEL READ")
							}

							d.Operation <- WriteOperationMessage{}
							d.Operation <- WriteOperationMessageFromSecondKafka{}
							fmt.Println("send WRITE operation")
						} else {
							if d.CancelWrite != nil {
								d.CancelWrite <- CancelWrite{}
								d.CancelWrite <- CancelWriteFromSecondKafka{}
								fmt.Println("send cancel WRITE")
							} else {
								d.CancelWrite = make(chan dispatcher.Operation)
								d.CancelWrite <- CancelWrite{}
								d.CancelWrite <- CancelWriteFromSecondKafka{}
								fmt.Println("CANCEL WRITE")
							}

							d.Operation <- ReadOperationMessage{}
							fmt.Println("send READ operation")

						}
					}()

				}
			case ReadOperationMessage:
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
									m, err := k.ReadMessage()
									if err != nil {
										log.Errorf("message not read", err)
									}
									fmt.Println(string(m))
									fmt.Println(string(mes))

									time.Sleep(time.Second)
								}
							}
						}
					}()
				}
			case WriteOperationMessage:
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

									msg, err := json.Marshal(map[string]string{
										"time": time.Now().String(),
									})
									if err != nil {
										log.Errorf("message not Send", err)
									}
									err = k.WriteMessage(msg)
									if err != nil {
										log.Errorf("message not Send", err)
									}
									fmt.Println("SEND MESSAGE")
									time.Sleep(time.Second)
								}
							}
						}
					}()
				}
			case WriteOperationMessageFromSecondKafka:
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

									//msg, err := json.Marshal(map[string]string{
									//	"time": time.Now().String(),
									//})
									if err != nil {
										log.Errorf("message not Send", err)
									}
									err = k2.WriteMessage([]byte(time.Now().String()))
									if err != nil {
										log.Errorf("message not Send", err)
									}
									fmt.Println("SEND MESSAGE FFROM K2")
									time.Sleep(time.Second * 3)
								}
							}
						}
					}()
				}
			}
		}

		dispatcher.Do(d, done)

		ManagerOfMessages(done, d)

		if isWatcher {
			d.Operation <- ReadOperationMessage{}
		} else {
			d.Operation <- WriteOperationMessage{}
			d.Operation <- WriteOperationMessageFromSecondKafka{}
		}

		<-s
		close(s)
		cnsl()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&isWatcher, "watcher", false, "User role for app")
	rootCmd.PersistentFlags().IntVar(&serverPort, "serverPort", 6774, "User role for app")
	rootCmd.PersistentFlags().IntVar(&clientPort, "clientPort", 8668, "User role for app")
}

func Execute() error {
	return rootCmd.Execute()
}

func ManagerOfMessages(ctx context.Context, d *dispatcher.Dispatcher) {
	go func(ctx context.Context) {
		roleTicker := time.NewTicker(time.Second * 30)
		//messageTicker := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-roleTicker.C:
				{
					fmt.Println("tick to change role")
					d.Operation <- WriteRole{}
				}
			case <-ctx.Done():
				{
					return
				}
			}
		}
	}(ctx)
}

func OB(dis *dispatcher.Dispatcher) func(msg string) {
	return func(msg string) {
		dis.Operation <- &ReadRole{Role: msg}
	}
}
