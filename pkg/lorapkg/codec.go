// Package lorapkg: encoding and decoding LoRaWAN payloads
package lorapkg

import (
	"fmt"

	"github.com/brocaar/lorawan"
)

type LoRaWANContext struct {
	DevAddr lorawan.DevAddr
	AppSKey lorawan.AES128Key
	NwkSKey lorawan.AES128Key
	FCnt    uint32
	FPort   uint8
}

func Encode(ctx LoRaWANContext, payload []byte) []byte {
	phy := &lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.ConfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: ctx.DevAddr,
				FCtrl:   lorawan.FCtrl{},
				FCnt:    ctx.FCnt,
			},
			FPort:      &ctx.FPort,
			FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: payload}},
		},
	}

	if err := phy.EncryptFRMPayload(ctx.AppSKey); err != nil {
		panic(err)
	}

	if err := phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ctx.NwkSKey, lorawan.AES128Key{}); err != nil {
		panic(err)
	}

	// b, err := phy.MarshalBinary()
	// if err != nil {
	// 	panic(err)
	// }

	str, err := phy.MarshalText()
	if err != nil {
		panic(err)
	}

	bytes, err := phy.MarshalBinary()
	if err != nil {
		panic(err)
	}

	phyJSON, err := phy.MarshalJSON()
	if err != nil {
		panic(err)
	}

	fmt.Println(string(str))
	fmt.Println(string(phyJSON))
	return bytes
}

func Decode(ctx LoRaWANContext, encoded []byte) []byte {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(encoded); err != nil {
		panic(err)
	}

	ok, err := phy.ValidateUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ctx.NwkSKey, lorawan.AES128Key{})
	if err != nil {
		panic(err)
	}
	if !ok {
		panic("invalid MIC")
	}

	if err := phy.DecodeFOptsToMACCommands(); err != nil {
		panic(err)
	}

	if err := phy.DecryptFRMPayload(ctx.AppSKey); err != nil {
		panic(err)
	}

	macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
	if !ok {
		panic("*MACPayload expected")
	}

	pl, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
	if !ok {
		panic("*DataPayload expected")
	}

	return pl.Bytes
}

// =======================================================================
// var (
// 	nwkSKey = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
// 	appSKey = [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
// 	appKey  = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
// )
//
// // Encode data
// func LorawanEncode(payload []byte) []byte {
// 	fPort := uint8(10)
//
// 	phy := &lorawan.PHYPayload{
// 		MHDR: lorawan.MHDR{
// 			MType: lorawan.ConfirmedDataUp,
// 			Major: lorawan.LoRaWANR1,
// 		},
// 		MACPayload: &lorawan.MACPayload{
// 			FHDR: lorawan.FHDR{
// 				DevAddr: lorawan.DevAddr([4]byte{1, 2, 3, 4}),
// 				FCtrl: lorawan.FCtrl{
// 					ADR:       false,
// 					ADRACKReq: false,
// 					ACK:       false,
// 				},
// 				FCnt: 0,
// 				FOpts: []lorawan.Payload{
// 					&lorawan.MACCommand{
// 						CID: lorawan.DevStatusAns,
// 						Payload: &lorawan.DevStatusAnsPayload{
// 							Battery: 115,
// 							Margin:  7,
// 						},
// 					},
// 				},
// 			},
// 			FPort:      &fPort,
// 			FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: payload}},
// 		},
// 	}
//
// 	if err := phy.EncryptFRMPayload(appSKey); err != nil {
// 		panic(err)
// 	}
//
// 	if err := phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, nwkSKey, lorawan.AES128Key{}); err != nil {
// 		panic(err)
// 	}
//
// 	str, err := phy.MarshalText()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	bytes, err := phy.MarshalBinary()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	phyJSON, err := phy.MarshalJSON()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	fmt.Println(string(str))
// 	fmt.Println(string(bytes))
// 	fmt.Println(string(phyJSON))
//
// 	return phyJSON
// }
//
// func LorawanDecode(encoded []byte) []byte {
// 	var phy lorawan.PHYPayload
// 	// use use UnmarshalBinary when decoding a byte-slice
// 	// if err := phy.UnmarshalText([]byte("gAQDAgEDAAAGcwcK4mTU9+EX0sA=")); err != nil {
// 	if err := phy.UnmarshalText(encoded); err != nil {
// 		panic(err)
// 	}
//
// 	ok, err := phy.ValidateUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, nwkSKey, lorawan.AES128Key{})
// 	if err != nil {
// 		panic(err)
// 	}
// 	if !ok {
// 		panic("invalid mic")
// 	}
//
// 	if err := phy.DecodeFOptsToMACCommands(); err != nil {
// 		panic(err)
// 	}
//
// 	phyJSON, err := phy.MarshalJSON()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	if err := phy.DecryptFRMPayload(appSKey); err != nil {
// 		panic(err)
// 	}
// 	macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
// 	if !ok {
// 		panic("*MACPayload expected")
// 	}
//
// 	pl, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
// 	if !ok {
// 		panic("*DataPayload expected")
// 	}
//
// 	fmt.Println(string(phyJSON))
// 	fmt.Println(string(pl.Bytes))
// 	return pl.Bytes
// }
//
// func LorawanJoinRequestSend() {
// 	phy := lorawan.PHYPayload{
// 		MHDR: lorawan.MHDR{
// 			MType: lorawan.JoinRequest,
// 			Major: lorawan.LoRaWANR1,
// 		},
// 		MACPayload: &lorawan.JoinRequestPayload{
// 			JoinEUI:  [8]byte{1, 1, 1, 1, 1, 1, 1, 1},
// 			DevEUI:   [8]byte{2, 2, 2, 2, 2, 2, 2, 2},
// 			DevNonce: 771,
// 		},
// 	}
//
// 	if err := phy.SetUplinkJoinMIC(appKey); err != nil {
// 		panic(err)
// 	}
//
// 	str, err := phy.MarshalText()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	bytes, err := phy.MarshalBinary()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	fmt.Println(string(str))
// 	fmt.Println(string(bytes))
// }
//
// func LorawanJoinRequestRead() {
// 	var phy lorawan.PHYPayload
// 	if err := phy.UnmarshalText([]byte("AAQDAgEEAwIBBQQDAgUEAwItEGqZDhI=")); err != nil {
// 		panic(err)
// 	}
//
// 	jrPL, ok := phy.MACPayload.(*lorawan.JoinRequestPayload)
// 	if !ok {
// 		panic("MACPayload must be a *JoinRequestPayload")
// 	}
//
// 	fmt.Println(phy.MHDR.MType)
// 	fmt.Println(jrPL.JoinEUI)
// 	fmt.Println(jrPL.DevEUI)
// 	fmt.Println(jrPL.DevNonce)
// }
//
// func LorawanJoinAcceptSend() {
// 	joinEUI := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}
// 	devNonce := lorawan.DevNonce(258)
//
// 	phy := lorawan.PHYPayload{
// 		MHDR: lorawan.MHDR{
// 			MType: lorawan.JoinAccept,
// 			Major: lorawan.LoRaWANR1,
// 		},
// 		MACPayload: &lorawan.JoinAcceptPayload{
// 			JoinNonce:  65793,
// 			HomeNetID:  [3]byte{2, 2, 2},
// 			DevAddr:    lorawan.DevAddr([4]byte{1, 2, 3, 4}),
// 			DLSettings: lorawan.DLSettings{RX2DataRate: 0, RX1DROffset: 0},
// 			RXDelay:    0,
// 		},
// 	}
//
// 	// set the MIC before encryption
// 	if err := phy.SetDownlinkJoinMIC(lorawan.JoinRequestType, joinEUI, devNonce, appKey); err != nil {
// 		panic(err)
// 	}
// 	if err := phy.EncryptJoinAcceptPayload(appKey); err != nil {
// 		panic(err)
// 	}
//
// 	str, err := phy.MarshalText()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	bytes, err := phy.MarshalBinary()
// 	if err != nil {
// 		panic(err)
// 	}
//
// 	fmt.Println(string(str))
// 	fmt.Println(string(bytes))
// }
