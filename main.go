package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
	"github.com/withObsrvr/pluginapi"
)

// Error types for the processor
type ErrorType string

const (
	ErrorTypeConfig     ErrorType = "config"
	ErrorTypeProcessing ErrorType = "processing"
	ErrorTypeParsing    ErrorType = "parsing"
	ErrorTypeValidation ErrorType = "validation"
)

type ErrorSeverity string

const (
	ErrorSeverityFatal   ErrorSeverity = "fatal"
	ErrorSeverityError   ErrorSeverity = "error"
	ErrorSeverityWarning ErrorSeverity = "warning"
)

// ProcessorError represents a rich error type for the processor
type ProcessorError struct {
	Err             error
	Type            ErrorType
	Severity        ErrorSeverity
	TransactionHash string
	LedgerSequence  uint32
	ContractID      string
	Context         map[string]interface{}
}

func NewProcessorError(err error, errType ErrorType, severity ErrorSeverity) *ProcessorError {
	return &ProcessorError{
		Err:      err,
		Type:     errType,
		Severity: severity,
		Context:  make(map[string]interface{}),
	}
}

func (e *ProcessorError) WithTransaction(txHash string) *ProcessorError {
	e.TransactionHash = txHash
	return e
}

func (e *ProcessorError) WithLedger(sequence uint32) *ProcessorError {
	e.LedgerSequence = sequence
	return e
}

func (e *ProcessorError) WithContract(contractID string) *ProcessorError {
	e.ContractID = contractID
	return e
}

func (e *ProcessorError) WithContext(key string, value interface{}) *ProcessorError {
	e.Context[key] = value
	return e
}

func (e *ProcessorError) Error() string {
	return fmt.Sprintf("%s error: %v", e.Type, e.Err)
}

// Event types
const (
	EventTypeSwap   = "swap"
	EventTypeAdd    = "add"
	EventTypeRemove = "remove"
)

// RouterEvent represents the base structure for router events with dual representation
type RouterEvent struct {
	// Raw data for archival
	RawTopics   []xdr.ScVal     `json:"raw_topics"`
	RawData     json.RawMessage `json:"raw_data"`
	RawEventXDR string          `json:"raw_event_xdr"`

	// Decoded data for usability
	Type           string    `json:"type"`
	Timestamp      time.Time `json:"timestamp"`
	LedgerSequence uint32    `json:"ledger_sequence"`
	ContractID     string    `json:"contract_id"`
	Account        string    `json:"account"`
	TokenA         string    `json:"token_a"`
	TokenB         string    `json:"token_b"`
	AmountA        string    `json:"amount_a"`
	AmountB        string    `json:"amount_b"`
	TxHash         string    `json:"tx_hash"`
}

// SwapEvent adds path information specific to swaps
type SwapEvent struct {
	RouterEvent
	Path []string `json:"path"`
}

// DiagnosticData contains additional diagnostic information about an event
type DiagnosticData struct {
	Event                    json.RawMessage `json:"event"`
	InSuccessfulContractCall bool            `json:"in_successful_contract_call"`
	RawXDR                   string          `json:"raw_xdr,omitempty"`
}

// ContractEvent represents a contract event with dual representation
type ContractEvent struct {
	// Transaction context
	TransactionHash   string    `json:"transaction_hash"`
	TransactionID     int64     `json:"transaction_id"`
	Successful        bool      `json:"successful"`
	LedgerSequence    uint32    `json:"ledger_sequence"`
	ClosedAt          time.Time `json:"closed_at"`
	NetworkPassphrase string    `json:"network_passphrase"`

	// Event context
	ContractID         string `json:"contract_id"`
	EventIndex         int    `json:"event_index"`
	OperationIndex     int    `json:"operation_index"`
	InSuccessfulTxCall bool   `json:"in_successful_tx_call"`

	// Event type information
	Type     string `json:"type"`
	TypeCode int32  `json:"type_code"`

	// Event data
	Topics        []TopicData     `json:"topics"`
	TopicsDecoded []TopicData     `json:"topics_decoded"`
	Data          json.RawMessage `json:"data"`
	DataDecoded   json.RawMessage `json:"data_decoded"`

	// Raw XDR for archival
	EventXDR string `json:"event_xdr"`

	// Additional diagnostic data
	DiagnosticEvents []DiagnosticData `json:"diagnostic_events,omitempty"`

	// Metadata for filtering
	Tags map[string]string `json:"tags,omitempty"`
}

// TopicData represents a structured topic with type information
type TopicData struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Str   string `json:"Str,omitempty"`
	Sym   string `json:"Sym,omitempty"`
}

// SoroswapRouterProcessor handles router events from Soroswap
type SoroswapRouterProcessor struct {
	consumers []pluginapi.Consumer
	mu        sync.RWMutex
	stats     struct {
		ProcessedEvents uint64
		SwapEvents      uint64
		AddEvents       uint64
		RemoveEvents    uint64
		LastEventTime   time.Time
		Errors          struct {
			Processing uint64
			Parsing    uint64
			Validation uint64
		}
	}
	startTime time.Time
}

// New creates a new instance of the plugin
func New() pluginapi.Plugin {
	return &SoroswapRouterProcessor{
		startTime: time.Now(),
	}
}

// Name returns the name of the plugin
func (p *SoroswapRouterProcessor) Name() string {
	return "flow/processor/soroswap-router"
}

// Version returns the version of the plugin
func (p *SoroswapRouterProcessor) Version() string {
	return "1.0.0"
}

// Type returns the type of the plugin
func (p *SoroswapRouterProcessor) Type() pluginapi.PluginType {
	return pluginapi.ProcessorPlugin
}

// Initialize sets up the processor with the provided configuration
func (p *SoroswapRouterProcessor) Initialize(config map[string]interface{}) error {
	if apiVersion, ok := config["flow_api_version"].(string); ok {
		if !isCompatibleVersion(apiVersion, "1.0.0") {
			log.Printf("Warning: Plugin was tested with Flow API v1.0.0, current version is %s", apiVersion)
		}
	}
	return nil
}

func isCompatibleVersion(current, minimum string) bool {
	// TODO: Implement proper version comparison
	return true
}

// RegisterConsumer registers a consumer with this processor
func (p *SoroswapRouterProcessor) RegisterConsumer(consumer pluginapi.Consumer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	log.Printf("Registering consumer: %s", consumer.Name())
	p.consumers = append(p.consumers, consumer)
}

// Helper function to encode contract IDs
func encodeContractID(contractId []byte) (string, error) {
	return strkey.Encode(strkey.VersionByteContract, contractId)
}

// Process handles router events
func (p *SoroswapRouterProcessor) Process(ctx context.Context, msg pluginapi.Message) error {
	log.Printf("SoroswapRouterProcessor: Processing message with payload type: %T", msg.Payload)

	var contractEvent ContractEvent
	switch payload := msg.Payload.(type) {
	case []byte:
		// Log the raw payload for debugging
		log.Printf("SoroswapRouterProcessor: Raw payload: %s", string(payload))
		if err := json.Unmarshal(payload, &contractEvent); err != nil {
			log.Printf("SoroswapRouterProcessor: Error decoding contract event from bytes: %v", err)
			return fmt.Errorf("error decoding contract event: %w", err)
		}
	case json.RawMessage:
		// Log the raw payload for debugging
		log.Printf("SoroswapRouterProcessor: Raw payload: %s", string(payload))
		if err := json.Unmarshal(payload, &contractEvent); err != nil {
			log.Printf("SoroswapRouterProcessor: Error decoding contract event from RawMessage: %v", err)
			return fmt.Errorf("error decoding contract event: %w", err)
		}
	case ContractEvent:
		contractEvent = payload
	default:
		log.Printf("SoroswapRouterProcessor: Unexpected payload type: %T", msg.Payload)
		return fmt.Errorf("unexpected payload type: %T", msg.Payload)
	}

	// Add debug logging for the entire contract event
	eventJson, _ := json.MarshalIndent(contractEvent, "", "  ")
	log.Printf("SoroswapRouterProcessor: Full contract event:\n%s", string(eventJson))

	// Check diagnostic events for Soroswap events
	for _, diagEvent := range contractEvent.DiagnosticEvents {
		// Log the diagnostic event for debugging
		log.Printf("Processing diagnostic event: %s", string(diagEvent.Event))

		var event struct {
			Event struct {
				ContractId []byte `json:"ContractId"`
				Type       int    `json:"Type"`
				Body       struct {
					V0 struct {
						Topics []struct {
							Type  int    `json:"Type"`
							Sym   string `json:"Sym"`
							Value string `json:"Value"`
						} `json:"Topics"`
						Data struct {
							Type int `json:"Type"`
							Map  []struct {
								Key struct {
									Type int    `json:"Type"`
									Sym  string `json:"Sym"`
								} `json:"Key"`
								Val struct {
									Type int `json:"Type"`
									I128 *struct {
										Hi int64  `json:"Hi"`
										Lo uint64 `json:"Lo"`
									} `json:"I128"`
									Address *struct {
										Type      int `json:"Type"`
										AccountId *struct {
											Ed25519 []byte `json:"Ed25519"`
										} `json:"AccountId"`
										ContractId []byte `json:"ContractId"`
									} `json:"Address"`
									Vec *[]struct {
										Type    int `json:"Type"`
										Address *struct {
											Type       int    `json:"Type"`
											ContractId []byte `json:"ContractId"`
										} `json:"Address"`
									} `json:"Vec"`
								} `json:"Val"`
							} `json:"Map"`
						} `json:"Data"`
					} `json:"V0"`
				} `json:"Body"`
			} `json:"Event"`
			InSuccessfulContractCall bool `json:"in_successful_contract_call"`
		}

		if err := json.Unmarshal(diagEvent.Event, &event); err != nil {
			log.Printf("Error parsing diagnostic event: %v", err)
			continue
		}

		// Check if this is a Soroswap event by looking at the topics
		if len(event.Event.Body.V0.Topics) >= 2 {
			// Log the topics for debugging
			for i, topic := range event.Event.Body.V0.Topics {
				log.Printf("Topic %d: Type=%d, Sym=%s, Value=%s", i, topic.Type, topic.Sym, topic.Value)
			}

			// Check for Soroswap event signature
			firstTopic := event.Event.Body.V0.Topics[0]
			secondTopic := event.Event.Body.V0.Topics[1]

			// Check if this is a Soroswap event
			isSoroswapEvent := false
			if firstTopic.Type == 14 { // Type 14 is Str
				if firstTopic.Value == "SoroswapRouter" || firstTopic.Value == "SoroswapPair" {
					isSoroswapEvent = true
				}
			}

			if !isSoroswapEvent {
				log.Printf("Not a Soroswap event: first topic type=%d value=%s", firstTopic.Type, firstTopic.Value)
				continue
			}

			// Determine event type
			var eventType string
			if secondTopic.Type == 15 { // Type 15 is Sym
				switch secondTopic.Sym {
				case "swap":
					eventType = EventTypeSwap
				case "add":
					eventType = EventTypeAdd
				case "remove":
					eventType = EventTypeRemove
				default:
					log.Printf("Unknown Soroswap event type: %s", secondTopic.Sym)
					continue
				}
			} else {
				log.Printf("Second topic is not a symbol: type=%d", secondTopic.Type)
				continue
			}

			// Create router event
			routerEvent := RouterEvent{
				Type:           eventType,
				Timestamp:      contractEvent.ClosedAt,
				LedgerSequence: contractEvent.LedgerSequence,
				ContractID:     contractEvent.ContractID,
				TxHash:         contractEvent.TransactionHash,
			}

			// Extract data from the map
			for _, entry := range event.Event.Body.V0.Data.Map {
				log.Printf("Processing map entry: key=%s", entry.Key.Sym)
				switch entry.Key.Sym {
				case "to":
					if entry.Val.Address != nil {
						if entry.Val.Address.AccountId != nil && entry.Val.Address.AccountId.Ed25519 != nil {
							if accountID, err := strkey.Encode(strkey.VersionByteAccountID, entry.Val.Address.AccountId.Ed25519); err == nil {
								routerEvent.Account = accountID
							}
						} else if entry.Val.Address.ContractId != nil {
							if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
								routerEvent.Account = contractID
							}
						}
					}
				case "amounts":
					if entry.Val.Vec != nil {
						// Parse the amounts vector
						var amounts []struct {
							Type int `json:"Type"`
							I128 *struct {
								Hi int64  `json:"Hi"`
								Lo uint64 `json:"Lo"`
							} `json:"I128"`
						}
						if amountsBytes, err := json.Marshal(entry.Val.Vec); err == nil {
							if err := json.Unmarshal(amountsBytes, &amounts); err == nil && len(amounts) >= 2 {
								// Extract amounts
								if amounts[0].I128 != nil {
									routerEvent.AmountA = fmt.Sprintf("%d", amounts[0].I128.Lo)
								}
								if amounts[1].I128 != nil {
									routerEvent.AmountB = fmt.Sprintf("%d", amounts[1].I128.Lo)
								}
							}
						}
					}
				case "path":
					if entry.Val.Vec != nil {
						path := *entry.Val.Vec
						if len(path) >= 2 {
							if path[0].Address != nil && path[0].Address.ContractId != nil {
								if contractID, err := encodeContractID(path[0].Address.ContractId); err == nil {
									routerEvent.TokenA = contractID
								}
							}
							if path[len(path)-1].Address != nil && path[len(path)-1].Address.ContractId != nil {
								if contractID, err := encodeContractID(path[len(path)-1].Address.ContractId); err == nil {
									routerEvent.TokenB = contractID
								}
							}
						}
					}
				case "amount0", "amount_a", "amount_0_in", "amount0_in":
					if entry.Val.I128 != nil {
						routerEvent.AmountA = fmt.Sprintf("%d", entry.Val.I128.Lo)
					}
				case "amount1", "amount_b", "amount_1_out", "amount1_out":
					if entry.Val.I128 != nil {
						routerEvent.AmountB = fmt.Sprintf("%d", entry.Val.I128.Lo)
					}
				case "token0", "token_a":
					if entry.Val.Address != nil && entry.Val.Address.ContractId != nil {
						if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
							routerEvent.TokenA = contractID
						}
					}
				case "token1", "token_b":
					if entry.Val.Address != nil && entry.Val.Address.ContractId != nil {
						if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
							routerEvent.TokenB = contractID
						}
					}
				}
			}

			// Log the extracted event data
			log.Printf("Extracted Soroswap event: %+v", routerEvent)

			// Update stats
			p.mu.Lock()
			p.stats.ProcessedEvents++
			switch eventType {
			case EventTypeSwap:
				p.stats.SwapEvents++
			case EventTypeAdd:
				p.stats.AddEvents++
			case EventTypeRemove:
				p.stats.RemoveEvents++
			}
			p.stats.LastEventTime = time.Now()
			p.mu.Unlock()

			// Forward the event to downstream consumers
			eventBytes, err := json.Marshal(routerEvent)
			if err != nil {
				return fmt.Errorf("error marshaling router event: %w", err)
			}

			log.Printf("Forwarding Soroswap %s event: %s (tokens: %s/%s, amounts: %s/%s)",
				eventType, routerEvent.ContractID, routerEvent.TokenA, routerEvent.TokenB,
				routerEvent.AmountA, routerEvent.AmountB)

			// Forward to all registered consumers
			log.Printf("Forwarding to %d consumers", len(p.consumers))
			for _, consumer := range p.consumers {
				log.Printf("Forwarding to consumer: %s", consumer.Name())
				if err := consumer.Process(ctx, pluginapi.Message{
					Payload:   eventBytes,
					Timestamp: time.Now(),
					Metadata: map[string]interface{}{
						"event_type": eventType,
						"data_type":  "router_events",
					},
				}); err != nil {
					return fmt.Errorf("error in consumer chain: %w", err)
				}
			}
		}
	}

	return nil
}

// recordError updates error statistics
func (p *SoroswapRouterProcessor) recordError(errType ErrorType) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch errType {
	case ErrorTypeProcessing:
		p.stats.Errors.Processing++
	case ErrorTypeParsing:
		p.stats.Errors.Parsing++
	case ErrorTypeValidation:
		p.stats.Errors.Validation++
	}
}

// GetStats returns the current processor statistics
func (p *SoroswapRouterProcessor) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return map[string]interface{}{
		"stats":     p.stats,
		"uptime":    time.Since(p.startTime).String(),
		"consumers": len(p.consumers),
	}
}

// Close handles cleanup
func (p *SoroswapRouterProcessor) Close() error {
	return nil
}

// GetSchemaDefinition returns the GraphQL schema for this processor
func (p *SoroswapRouterProcessor) GetSchemaDefinition() string {
	return `
type RouterEvent {
    type: String!
    timestamp: String!
    ledger_sequence: Int!
    contract_id: String!
    account: String
    token_a: String
    token_b: String
    amount_a: String
    amount_b: String
    tx_hash: String!
}

type SwapEvent {
    router_event: RouterEvent!
    path: [String!]!
}

type Query {
    getRouterEvents(contract_id: String): [RouterEvent!]!
    getSwapEvents(contract_id: String): [SwapEvent!]!
}
`
}

// GetQueryDefinitions returns the GraphQL query definitions
func (p *SoroswapRouterProcessor) GetQueryDefinitions() string {
	return `
    getRouterEvents(contract_id: String): [RouterEvent!]!
    getSwapEvents(contract_id: String): [SwapEvent!]!
`
}

// processEventWithType handles processing of an event with a known type
func (p *SoroswapRouterProcessor) processEventWithType(ctx context.Context, contractEvent ContractEvent, eventType string) error {
	// Parse the event data
	var eventData struct {
		V0 struct {
			Data struct {
				Map []struct {
					Key struct {
						Sym string `json:"Sym"`
					} `json:"Key"`
					Val struct {
						Address struct {
							ContractId []byte `json:"ContractId"`
							AccountId  *struct {
								Ed25519 []byte `json:"Ed25519"`
							} `json:"AccountId"`
						} `json:"Address"`
						I128 struct {
							Lo uint64 `json:"Lo"`
						} `json:"I128"`
						Bytes []byte `json:"Bytes"`
						Vec   []struct {
							Address struct {
								ContractId []byte `json:"ContractId"`
							} `json:"Address"`
							I128 struct {
								Lo uint64 `json:"Lo"`
							} `json:"I128"`
						} `json:"Vec"`
					} `json:"Val"`
				} `json:"Map"`
			} `json:"Data"`
		} `json:"V0"`
	}

	if err := json.Unmarshal(contractEvent.Data, &eventData); err != nil {
		log.Printf("SoroswapRouterProcessor: Error parsing router event data: %v", err)
		return fmt.Errorf("error parsing router event data: %w", err)
	}

	// Create base router event
	routerEvent := RouterEvent{
		Type:           eventType,
		Timestamp:      contractEvent.ClosedAt,
		LedgerSequence: contractEvent.LedgerSequence,
		ContractID:     contractEvent.ContractID,
		TxHash:         contractEvent.TransactionHash,
	}

	// Extract data based on event type
	for _, entry := range eventData.V0.Data.Map {
		switch entry.Key.Sym {
		case "path":
			// Handle swap event path
			if entry.Val.Vec != nil && len(entry.Val.Vec) >= 2 {
				if entry.Val.Vec[0].Address.ContractId != nil {
					if contractID, err := encodeContractID(entry.Val.Vec[0].Address.ContractId); err == nil {
						routerEvent.TokenA = contractID
					}
				}
				if entry.Val.Vec[len(entry.Val.Vec)-1].Address.ContractId != nil {
					if contractID, err := encodeContractID(entry.Val.Vec[len(entry.Val.Vec)-1].Address.ContractId); err == nil {
						routerEvent.TokenB = contractID
					}
				}
			}
		case "amounts":
			// Handle swap event amounts
			if entry.Val.Vec != nil {
				// Parse the amounts vector
				var amounts []struct {
					Type int `json:"Type"`
					I128 *struct {
						Hi int64  `json:"Hi"`
						Lo uint64 `json:"Lo"`
					} `json:"I128"`
				}
				if amountsBytes, err := json.Marshal(entry.Val.Vec); err == nil {
					if err := json.Unmarshal(amountsBytes, &amounts); err == nil && len(amounts) >= 2 {
						// Extract amounts
						if amounts[0].I128 != nil {
							routerEvent.AmountA = fmt.Sprintf("%d", amounts[0].I128.Lo)
						}
						if amounts[1].I128 != nil {
							routerEvent.AmountB = fmt.Sprintf("%d", amounts[1].I128.Lo)
						}
					}
				}
			}
		case "token0", "token_a":
			// Handle add/remove event token A
			if entry.Val.Address.ContractId != nil {
				if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
					routerEvent.TokenA = contractID
				}
			}
		case "token1", "token_b":
			// Handle add/remove event token B
			if entry.Val.Address.ContractId != nil {
				if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
					routerEvent.TokenB = contractID
				}
			}
		case "amount0", "amount_a":
			// Handle add/remove event amount A
			routerEvent.AmountA = fmt.Sprintf("%d", entry.Val.I128.Lo)
		case "amount1", "amount_b":
			// Handle add/remove event amount B
			routerEvent.AmountB = fmt.Sprintf("%d", entry.Val.I128.Lo)
		case "to":
			if entry.Val.Address.AccountId != nil && entry.Val.Address.AccountId.Ed25519 != nil {
				if accountID, err := strkey.Encode(strkey.VersionByteAccountID, entry.Val.Address.AccountId.Ed25519); err == nil {
					routerEvent.Account = accountID
				}
			} else if entry.Val.Address.ContractId != nil {
				if contractID, err := encodeContractID(entry.Val.Address.ContractId); err == nil {
					routerEvent.Account = contractID
				}
			}
		}
	}

	// Add validation before forwarding the event
	if routerEvent.TokenA == "" || routerEvent.TokenB == "" ||
		routerEvent.AmountA == "" || routerEvent.AmountB == "" {
		log.Printf("Warning: Incomplete event data detected: %+v", routerEvent)
	}

	// Add debug logging after data extraction
	log.Printf("Extracted event data: %+v", routerEvent)
	log.Printf("Router Event Details:")
	log.Printf("  Type: %s", routerEvent.Type)
	log.Printf("  Account: %s", routerEvent.Account)
	log.Printf("  TokenA: %s", routerEvent.TokenA)
	log.Printf("  TokenB: %s", routerEvent.TokenB)
	log.Printf("  AmountA: %s", routerEvent.AmountA)
	log.Printf("  AmountB: %s", routerEvent.AmountB)
	log.Printf("  TxHash: %s", routerEvent.TxHash)
	log.Printf("  ContractID: %s", routerEvent.ContractID)

	// Update stats
	p.mu.Lock()
	p.stats.ProcessedEvents++
	switch eventType {
	case EventTypeSwap:
		p.stats.SwapEvents++
	case EventTypeAdd:
		p.stats.AddEvents++
	case EventTypeRemove:
		p.stats.RemoveEvents++
	}
	p.stats.LastEventTime = time.Now()
	p.mu.Unlock()

	// Forward the event to downstream consumers
	eventBytes, err := json.Marshal(routerEvent)
	if err != nil {
		return fmt.Errorf("error marshaling router event: %w", err)
	}

	log.Printf("Processing %s event: %s (tokens: %s/%s, amounts: %s/%s)",
		eventType, routerEvent.ContractID, routerEvent.TokenA, routerEvent.TokenB,
		routerEvent.AmountA, routerEvent.AmountB)

	// Forward to all registered consumers
	log.Printf("Forwarding event to %d consumers", len(p.consumers))
	for _, consumer := range p.consumers {
		log.Printf("Forwarding to consumer: %s", consumer.Name())
		if err := consumer.Process(ctx, pluginapi.Message{
			Payload:   eventBytes,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"event_type": eventType,
				"data_type":  "router_events",
			},
		}); err != nil {
			return fmt.Errorf("error in consumer chain: %w", err)
		}
	}

	return nil
}
