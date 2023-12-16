package core

import (
	"encoding/base64"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/DefiantLabs/cosmos-indexer/db"
	"github.com/DefiantLabs/cosmos-indexer/db/models"
	"github.com/DefiantLabs/cosmos-indexer/filter"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
)

// TODO: This is a stub, for use when we have begin blocker events in generic manner
// var (
// 	beginBlockerEventTypeHandlers = map[string][]func() eventTypes.CosmosEvent{}
// 	endBlockerEventTypeHandlers   = map[string][]func() eventTypes.CosmosEvent{}
// )

func ChainSpecificEndBlockerEventTypeHandlerBootstrap(chainID string) {
	// Stub, for use when we have begin blocker events
}

func ChainSpecificBeginBlockerEventTypeHandlerBootstrap(chainID string) {
	// Stub, for use when we have begin blocker events
}

func ProcessRPCBlockResults(blockResults *ctypes.ResultBlockResults) (*db.BlockDBWrapper, error) {
	var blockDBWrapper db.BlockDBWrapper

	blockDBWrapper.Block = &models.Block{
		Height: blockResults.Height,
	}

	blockDBWrapper.UniqueBlockEventAttributeKeys = make(map[string]models.BlockEventAttributeKey)
	blockDBWrapper.UniqueBlockEventTypes = make(map[string]models.BlockEventType)

	var err error
	blockDBWrapper.BeginBlockEvents, err = ProcessRPCBlockEvents(blockDBWrapper.Block, blockResults.BeginBlockEvents, models.BeginBlockEvent, blockDBWrapper.UniqueBlockEventTypes, blockDBWrapper.UniqueBlockEventAttributeKeys)

	if err != nil {
		return nil, err
	}

	blockDBWrapper.EndBlockEvents, err = ProcessRPCBlockEvents(blockDBWrapper.Block, blockResults.EndBlockEvents, models.EndBlockEvent, blockDBWrapper.UniqueBlockEventTypes, blockDBWrapper.UniqueBlockEventAttributeKeys)

	if err != nil {
		return nil, err
	}

	return &blockDBWrapper, nil
}

func ProcessRPCBlockEvents(block *models.Block, blockEvents []abci.Event, blockLifecyclePosition models.BlockLifecyclePosition, uniqueEventTypes map[string]models.BlockEventType, uniqueAttributeKeys map[string]models.BlockEventAttributeKey) ([]db.BlockEventDBWrapper, error) {
	beginBlockEvents := make([]db.BlockEventDBWrapper, len(blockEvents))

	for index, event := range blockEvents {
		eventType := models.BlockEventType{
			Type: event.Type,
		}
		beginBlockEvents[index].BlockEvent = models.BlockEvent{
			Index:             uint64(index),
			LifecyclePosition: blockLifecyclePosition,
			Block:             *block,
			BlockEventType:    eventType,
		}

		uniqueEventTypes[event.Type] = eventType

		beginBlockEvents[index].Attributes = make([]models.BlockEventAttribute, len(event.Attributes))

		for attrIndex, attribute := range event.Attributes {

			// Should we even be decoding these from base64? What are the implications?
			valueBytes, err := base64.StdEncoding.DecodeString(attribute.Value)
			if err != nil {
				return nil, err
			}

			keyBytes, err := base64.StdEncoding.DecodeString(attribute.Key)
			if err != nil {
				return nil, err
			}

			key := models.BlockEventAttributeKey{
				Key: string(keyBytes),
			}

			beginBlockEvents[index].Attributes[attrIndex] = models.BlockEventAttribute{
				Value:                  string(valueBytes),
				BlockEventAttributeKey: key,
				Index:                  uint64(attrIndex),
			}

			uniqueAttributeKeys[key.Key] = key

		}

	}

	return beginBlockEvents, nil
}

func FilterRPCBlockEvents(blockEvents []db.BlockEventDBWrapper, filterRegistry filter.StaticBlockEventFilterRegistry) ([]db.BlockEventDBWrapper, error) {

	// If there are no filters, just return the block events
	if len(filterRegistry.BlockEventFilters) == 0 && len(filterRegistry.RollingWindowEventFilters) == 0 {
		return blockEvents, nil
	}

	filterIndexes := make(map[int]bool)

	// If filters are defined, we treat filters as a whitelist, and only include block events that match the filters and are allowed
	// Filters are evaluated in order, and the first filter that matches is the one that is used. Single block event filters are preferred in ordering.
	for index, blockEvent := range blockEvents {
		filterEvent := filter.FilterEventData{
			Event:      blockEvent.BlockEvent,
			Attributes: blockEvent.Attributes,
		}

		for _, filter := range filterRegistry.BlockEventFilters {
			patternMatch, err := filter.EventMatches(filterEvent)
			if err != nil {
				return nil, err
			}
			if patternMatch {
				filterIndexes[index] = filter.IncludeMatch()
			}
		}

		if _, inMap := filterIndexes[index]; !inMap {
			for _, rollingWindowFilter := range filterRegistry.RollingWindowEventFilters {
				if index+rollingWindowFilter.RollingWindowLength() <= len(blockEvents) {
					blockEventSlice := blockEvents[index : index+rollingWindowFilter.RollingWindowLength()]

					filterEvents := make([]filter.FilterEventData, len(blockEventSlice))

					for index, blockEvent := range blockEventSlice {
						filterEvents[index] = filter.FilterEventData{
							Event:      blockEvent.BlockEvent,
							Attributes: blockEvent.Attributes,
						}
					}

					patternMatches, err := rollingWindowFilter.EventsMatch(filterEvents)

					if err != nil {
						return nil, err
					}

					if patternMatches {
						filterIndexes[index] = rollingWindowFilter.IncludeMatches()
						break
					}
				}

			}
		}
	}

	// Filter the block events based on the indexes that matched the registered patterns
	filteredBlockEvents := make([]db.BlockEventDBWrapper, 0)

	for index, blockEvent := range blockEvents {
		if filterIndexes[index] {
			filteredBlockEvents = append(filteredBlockEvents, blockEvent)
		}
	}

	return filteredBlockEvents, nil
}
