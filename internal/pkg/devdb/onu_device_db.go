/*
 * Copyright 2020-present Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

//Package devdb provides access to internal device ME DB
package devdb

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"sync"

	me "github.com/opencord/omci-lib-go/v2/generated"
	"github.com/opencord/voltha-lib-go/v7/pkg/log"
)

type meDbMap map[me.ClassID]map[uint16]me.AttributeValueMap

// UnknownMeName type to be used for unknown ME names
type UnknownMeName string

// allowed values for type UnknownMeName
const (
	CUnknownItuG988ManagedEntity        = "UnknownItuG988ManagedEntity"
	CUnknownVendorSpecificManagedEntity = "UnknownVendorSpecificManagedEntity"
)

type unknownMeAttribs struct {
	AttribMask  string `json:"AttributeMask"`
	AttribBytes string `json:"AttributeBytes"`
}
type unknownMeDbMap map[UnknownMeName]map[me.ClassID]map[uint16]unknownMeAttribs

//OnuDeviceDB structure holds information about ME's
type OnuDeviceDB struct {
	ctx         context.Context
	deviceID    string
	MeDb        meDbMap
	meDbLock    sync.RWMutex
	UnknownMeDb unknownMeDbMap
}

//NewOnuDeviceDB returns a new instance for a specific ONU_Device_Entry
func NewOnuDeviceDB(ctx context.Context, aDeviceID string) *OnuDeviceDB {
	logger.Debugw(ctx, "Init OnuDeviceDB for:", log.Fields{"device-id": aDeviceID})
	var OnuDeviceDB OnuDeviceDB
	OnuDeviceDB.ctx = ctx
	OnuDeviceDB.deviceID = aDeviceID
	OnuDeviceDB.MeDb = make(meDbMap)
	OnuDeviceDB.UnknownMeDb = make(unknownMeDbMap)

	return &OnuDeviceDB
}

//PutMe puts an ME instance into internal ONU DB
func (OnuDeviceDB *OnuDeviceDB) PutMe(ctx context.Context, meClassID me.ClassID, meEntityID uint16, meAttributes me.AttributeValueMap) {
	OnuDeviceDB.meDbLock.Lock()
	defer OnuDeviceDB.meDbLock.Unlock()
	//filter out the OnuData
	if me.OnuDataClassID == meClassID {
		return
	}
	_, ok := OnuDeviceDB.MeDb[meClassID]
	if !ok {
		logger.Debugw(ctx, "meClassID not found - add to db :", log.Fields{"device-id": OnuDeviceDB.deviceID})
		OnuDeviceDB.MeDb[meClassID] = make(map[uint16]me.AttributeValueMap)
		OnuDeviceDB.MeDb[meClassID][meEntityID] = make(me.AttributeValueMap)
		OnuDeviceDB.MeDb[meClassID][meEntityID] = meAttributes
	} else {
		meAttribs, ok := OnuDeviceDB.MeDb[meClassID][meEntityID]
		if !ok {
			OnuDeviceDB.MeDb[meClassID][meEntityID] = make(me.AttributeValueMap)
			OnuDeviceDB.MeDb[meClassID][meEntityID] = meAttributes
		} else {
			for k, v := range meAttributes {
				meAttribs[k] = v
			}
			OnuDeviceDB.MeDb[meClassID][meEntityID] = meAttribs
		}
	}
}

//GetMe returns an ME instance from internal ONU DB
func (OnuDeviceDB *OnuDeviceDB) GetMe(meClassID me.ClassID, meEntityID uint16) me.AttributeValueMap {
	OnuDeviceDB.meDbLock.RLock()
	defer OnuDeviceDB.meDbLock.RUnlock()
	if meAttributes, present := OnuDeviceDB.MeDb[meClassID][meEntityID]; present {
		/* verbose logging, avoid in >= debug level
		logger.Debugw(ctx,"ME found:", log.Fields{"meClassID": meClassID, "meEntityID": meEntityID, "meAttributes": meAttributes,
			"device-id": OnuDeviceDB.deviceID})
		*/
		return meAttributes
	}
	return nil
}

//GetUint32Attrib converts JSON numbers to uint32
func (OnuDeviceDB *OnuDeviceDB) GetUint32Attrib(meAttribute interface{}) (uint32, error) {

	switch reflect.TypeOf(meAttribute).Kind() {
	case reflect.Float64:
		//JSON numbers by default are unmarshaled into values of float64 type if type information is not present
		return uint32(meAttribute.(float64)), nil
	case reflect.Uint32:
		return meAttribute.(uint32), nil
	default:
		return uint32(0), fmt.Errorf(fmt.Sprintf("wrong-interface-type-%v-received-for-device-%s", reflect.TypeOf(meAttribute).Kind(), OnuDeviceDB.deviceID))
	}
}

//GetUint16Attrib converts JSON numbers to uint16
func (OnuDeviceDB *OnuDeviceDB) GetUint16Attrib(meAttribute interface{}) (uint16, error) {

	switch reflect.TypeOf(meAttribute).Kind() {
	case reflect.Float64:
		//JSON numbers by default are unmarshaled into values of float64 type if type information is not present
		return uint16(meAttribute.(float64)), nil
	case reflect.Uint16:
		return meAttribute.(uint16), nil
	default:
		return uint16(0), fmt.Errorf(fmt.Sprintf("wrong-interface-type-%v-received-for-device-%s", reflect.TypeOf(meAttribute).Kind(), OnuDeviceDB.deviceID))
	}
}

//GetSortedInstKeys returns a sorted list of all instances of an ME
func (OnuDeviceDB *OnuDeviceDB) GetSortedInstKeys(ctx context.Context, meClassID me.ClassID) []uint16 {

	var meInstKeys []uint16
	OnuDeviceDB.meDbLock.RLock()
	defer OnuDeviceDB.meDbLock.RUnlock()
	meInstMap := OnuDeviceDB.MeDb[meClassID]

	for k := range meInstMap {
		meInstKeys = append(meInstKeys, k)
	}
	logger.Debugw(ctx, "meInstKeys - input order :", log.Fields{"meInstKeys": meInstKeys}) //TODO: delete the line after test phase!
	sort.Slice(meInstKeys, func(i, j int) bool { return meInstKeys[i] < meInstKeys[j] })
	logger.Debugw(ctx, "meInstKeys - output order :", log.Fields{"meInstKeys": meInstKeys}) //TODO: delete the line after test phase!
	return meInstKeys
}

//LogMeDb logs content of internal ONU DB
func (OnuDeviceDB *OnuDeviceDB) LogMeDb(ctx context.Context) {
	logger.Debugw(ctx, "ME instances stored for :", log.Fields{"device-id": OnuDeviceDB.deviceID})
	for meClassID, meInstMap := range OnuDeviceDB.MeDb {
		for meEntityID, meAttribs := range meInstMap {
			logger.Debugw(ctx, "ME instance: ", log.Fields{"meClassID": meClassID, "meEntityID": meEntityID, "meAttribs": meAttribs,
				"device-id": OnuDeviceDB.deviceID})
		}
	}
}

//PutUnknownMe puts an unknown ME instance into internal ONU DB
func (OnuDeviceDB *OnuDeviceDB) PutUnknownMe(ctx context.Context, aMeName UnknownMeName, aMeClassID me.ClassID, aMeEntityID uint16,
	aMeAttributeMask uint16, aMePayload []byte) {

	meAttribMaskStr := strconv.FormatUint(uint64(aMeAttributeMask), 16)
	attribs := unknownMeAttribs{meAttribMaskStr, hex.EncodeToString(aMePayload)}

	_, ok := OnuDeviceDB.UnknownMeDb[aMeName]
	if !ok {
		OnuDeviceDB.UnknownMeDb[aMeName] = make(map[me.ClassID]map[uint16]unknownMeAttribs)
		OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID] = make(map[uint16]unknownMeAttribs)
		OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID][aMeEntityID] = attribs
	} else {
		_, ok := OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID]
		if !ok {
			OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID] = make(map[uint16]unknownMeAttribs)
			OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID][aMeEntityID] = attribs
		} else {
			_, ok := OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID][aMeEntityID]
			if !ok {
				OnuDeviceDB.UnknownMeDb[aMeName][aMeClassID][aMeEntityID] = attribs
			}
		}
	}
}
