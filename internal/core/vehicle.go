// Package core implements the Vehicle agent
package core

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

// VehicleState định nghĩa trạng thái của xe
type VehicleState int

const (
	StateIdle    VehicleState = 0 // Chờ Beacon, chưa có slot
	StateJoining VehicleState = 1 // Đã gửi Hello, chờ Beacon tiếp theo để confirm slot
	StateSending VehicleState = 2 // Đã có slot, gửi Telemetry định kỳ
)

// Vehicle là đại diện cho thiết bị thuyền/xe
type Vehicle struct {
	ID      string
	lora    *device.Lora
	arduino *device.Arduino // Có thể nil nếu dùng dữ liệu giả lập

	state          VehicleState
	currentGateway string
	assignedSlot   int

	// Cache dữ liệu telemetry mới nhất từ Arduino
	mutexTelemetry sync.Mutex
	lastTelemetry  model.ArduinoData

	mutexOffset sync.Mutex
	bestOffset  int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewVehicle tạo một Vehicle mới
func NewVehicle(
	id string,
	loraDev string, loraBaud int,
	arduinoDev string, arduinoBaud int,
) *Vehicle {
	lora := device.NewLora(loraDev, loraBaud)
	v := &Vehicle{
		ID:           id,
		lora:         lora,
		state:        StateIdle,
		assignedSlot: -1,
		bestOffset:   math.MaxInt64,
	}
	if arduinoDev != "" {
		v.arduino = device.NewArduino(arduinoDev, arduinoBaud)
	}
	return v
}

func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	slog.Info("[Vehicle] Started",
		"vehicle", v.ID)

	// 1. Goroutine đọc Arduino liên tục để update lastTelem
	if v.arduino != nil {
		v.wg.Add(1)
		go v.arduinoLoop(ctx)
	}

	// 2. Goroutine chính: LoRa Loop (State Machine)
	v.wg.Add(1)
	go v.loraLoop(ctx)

	return nil
}

func (v *Vehicle) Stop() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		if err := v.lora.Close(); err != nil {
			slog.Warn("[Vehicle] Failed to close LoRa device",
				"vehicle", v.ID, "error", err)
		}
	}
	if v.arduino != nil {
		if err := v.arduino.Close(); err != nil {
			slog.Warn("[Vehicle] Failed to close Arduino device",
				"vehicle", v.ID, "error", err)
		}
	}
	v.wg.Wait()
	slog.Info("[Vehicle] Stopped",
		"vehicle", v.ID)
}

// arduinoLoop đọc dữ liệu từ Arduino/Simulator
func (v *Vehicle) arduinoLoop(ctx context.Context) {
	defer v.wg.Done()
	dataCh := make(chan model.ArduinoData, 5)
	stop, err := v.arduino.Read(dataCh)
	if err != nil {
		slog.Warn("[Vehicle] Failed to read Arduino",
			"vehicle", v.ID, "err", err)
		return
	}
	defer stop()

	// Khởi tạo data giả nếu arduino nil
	if v.arduino == nil {
		v.mutexTelemetry.Lock()
		v.lastTelemetry = model.ArduinoData{
			Latitude:    21.0532 + rand.Float64()*0.001,
			Longitude:   105.8261 + rand.Float64()*0.001,
			CurrentHead: 0,    // + rand.Int63n(361),
			TargetHead:  0,    // + rand.Int63n(361),
			LeftSpeed:   1000, // + rand.Int63n(1000),
			RightSpeed:  1000, // + rand.Int63n(1000),
		}
		v.mutexTelemetry.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dataCh:
			if !ok {
				return
			}
			v.mutexTelemetry.Lock()
			v.lastTelemetry = data
			v.mutexTelemetry.Unlock()
			// slog.Info("Vehicle received arduino data",
			// 	"vehicle", v.ID,
			// 	"lat", data.Latitude,
			// 	"lon", data.Longitude,
			// 	"curHead", data.CurrentHead,
			// 	"tarHead", data.TargetHead,
			// 	"leftSpeed", data.LeftSpeed,
			// 	"rightSpeed", data.RightSpeed,
			// )
		}
	}
}

// loraLoop là State Machine chính của Vehicle
func (v *Vehicle) loraLoop(ctx context.Context) {
	defer v.wg.Done()

	// Sử dụng giá trị mặc định cho timeout chờ beacon (ví dụ: 5 giây)
	var lastBeaconTime time.Time
	beaconTimeout := 20 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		frame, err := v.lora.ReadLine(beaconTimeout)
		if err != nil {
			// Nếu timeout hoặc lỗi sau khi đã từng có session -> Reset về IDLE
			if err == device.ErrTimeout {
				// if we had a previous beacon and too long passed -> reset
				if !lastBeaconTime.IsZero() && time.Since(lastBeaconTime) > 2*beaconTimeout {
					if v.state != StateIdle {
						slog.Warn("[Vehicle] Lost beacon connection => State: IDLE",
							"vehicle", v.ID, "state", v.state)
					}
					v.state = StateIdle
					v.currentGateway = ""
					v.assignedSlot = -1
				}
				continue
			}
			// Nếu đã Idle thì cứ tiếp tục lắng nghe
			slog.Error("[Vehicle] Failed to read Lora",
				"vehicle", v.ID, "state", v.state, "err", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Phân loại gói tin
		var generic map[string]any
		if err := cbor.Unmarshal(frame, &generic); err != nil {
			slog.Warn("[Vehicle] Received unknown or corrupted CBOR packet",
				"vehicle", v.ID, "state", v.state, "err", err)
			continue
		}

		msgType, ok := generic["type"].(string)
		if !ok {
			slog.Warn("[Vehicle] Packet type missing",
				"vehicle", v.ID, "state", v.state)
			continue
		}

		switch msgType {
		case model.PacketBeacon:
			var beacon model.BeaconMessage
			if err := cbor.Unmarshal(frame, &beacon); err != nil {
				// Có thể là packet Control hoặc nhiễu
				slog.Info("[Vehicle] Received non-beacon frame or corrupted beacon",
					"vehicle", v.ID, "state", v.state, "error", err)
				continue
			}
			slog.Info("[Vehicle] Received BEACON",
				"vehicle", v.ID, "state", v.state, "source", beacon.GatewayAddress)

			// record last beacon time and timestamp (ms)
			lastBeaconTime = time.Now()
			v.handleBeacon(ctx, beacon)
		case model.PacketControl:
			if v.state == StateSending {
				var control model.ControlData
				if err := cbor.Unmarshal(frame, &control); err != nil {
					// Có thể là packet Control hoặc nhiễu
					slog.Info("[Vehicle] Received non-control frame or corrupted control",
						"vehicle", v.ID, "state", v.state, "error", err)
					continue
				}
				slog.Info("[Vehicle] Received CONTROL",
					"vehicle", v.ID, "state", v.state)
				if control.VehicleID != v.ID {
					slog.Debug("[Vehicle] Not target control",
						"vehicle", v.ID, "target", control.VehicleID)
					continue
				}
				arduinoControl := fmt.Sprintf("%d,%.6f,%.6f,%.6f,%.6f,%.6f",
					control.Speed, control.Latitude, control.Longitude,
					control.Kp, control.Ki, control.Kd)
				// Forward control data to Arduino
				if err := v.arduino.Write(arduinoControl); err != nil {
					slog.Error("[Vehicle] Failed to forward control to Arduino",
						"vehicle", v.ID, "state", v.state, "error", err)
				} else {
					slog.Info("[Vehicle] Forwarded CONTROL to Arduino",
						"vehicle", v.ID, "state", v.state)
				}
			}
		default:
			slog.Info("[Vehicle] Received unhandled message type",
				"vehicle", v.ID, "state", v.state, "type", msgType)
		}
	}
}

func (v *Vehicle) handleBeacon(ctx context.Context, b model.BeaconMessage) {
	// 1. CHECK ROAMING ĐẦU TIÊN
	// Để quyết định xem có cần reset bộ lọc đồng bộ hay không
	isRoaming := v.currentGateway != "" && v.currentGateway != b.GatewayAddress
	v.mutexOffset.Lock()
	if isRoaming {
		slog.Info("[Vehicle] Roaming detected - Resetting Sync",
			"old", v.currentGateway, "new", b.GatewayAddress)
		// Reset về trạng thái chưa đồng bộ để bắt đầu tính lại từ đầu với Gateway mới
		v.bestOffset = math.MaxInt64
	}

	// 2. TÍNH TOÁN ĐỒNG BỘ (SYNC)
	newOffset := time.Now().UnixNano()/int64(time.Millisecond) - b.CycleStartMs
	v.bestOffset = min(v.bestOffset, newOffset)
	cycleStart := time.UnixMilli(b.CycleStartMs)
	localCycleStart := cycleStart.Add(time.Duration(v.bestOffset) * time.Millisecond)
	v.mutexOffset.Unlock()

	// 3. TÍNH TOÁN CÁC MỐC THỜI GIAN (Dựa trên localCycleStart ĐÚNG)
	controlStart := localCycleStart.Add(time.Duration(b.BeaconWindowMs) * time.Millisecond)
	registerStart := controlStart.Add(time.Duration(b.ControlWindowMs+b.GuardTimeMs) * time.Millisecond)
	registerTime := registerStart.Add(time.Duration(rand.Int63n(b.RegisterWindowMs/2)) * time.Millisecond)

	// 4. Logic Roaming: Nếu gateway ID khác với hiện tại và đã có slot
	if isRoaming {
		v.currentGateway = b.GatewayAddress
		v.state = StateIdle // Reset về Idle để đăng ký lại với Gateway mới
		v.assignedSlot = -1
		slog.Info("[Vehicle] Roaming detected => Preparing to switch",
			"vehicle", v.ID, "state", v.state, "old", v.currentGateway, "new", b.GatewayAddress)

		// sleep until register window
		v.sleepUntil(ctx, registerTime)
		// Re-check: maybe beacon or another goroutine assigned slot
		if v.assignedSlot != -1 {
			// already have slot -> go to sending
			v.state = StateSending
			slog.Info(
				"[Vehicle] Already assigned slot while waiting",
				"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot,
			)
			go v.performTDMA(ctx, b, localCycleStart)
			return
		}

		// Gửi lại Hello
		hello := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(hello)
		if err := v.lora.WriteLine(payload); err != nil {
			slog.Warn("[Vehicle] Failed to send HELLO during roaming",
				"vehicle", v.ID, "state", v.state, "err", err)
		} else {
			v.state = StateJoining
			slog.Info("[Vehicle] Sent HELLO (Roaming)",
				"vehicle", v.ID, "state", v.state)
		}
		return
	}

	// 5. CẬP NHẬT TRẠNG THÁI BÌNH THƯỜNG
	v.currentGateway = b.GatewayAddress
	switch v.state {
	case StateIdle:
		// sleep until register window
		v.sleepUntil(ctx, registerTime)
		// Re-check: maybe beacon or another goroutine assigned slot
		if v.assignedSlot != -1 {
			// already have slot -> go to sending
			v.state = StateSending
			slog.Info(
				"[Vehicle] Already assigned slot while waiting",
				"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot,
			)
			go v.performTDMA(ctx, b, localCycleStart)
			return
		}

		// Gửi Hello
		msg := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(msg)
		if err := v.lora.WriteLine(payload); err != nil {
			slog.Warn("[Vehicle] Failed to write HELLO",
				"vehicle", v.ID, "state", v.state, "err", err)
		} else {
			v.state = StateJoining
			slog.Info("[Vehicle] Sent HELLO",
				"vehicle", v.ID, "state", v.state)
		}

	case StateJoining:
		// Trạng thái CHUẨN BỊ GỬI: Kiểm tra xem trong Beacon mới có Slot cho mình chưa
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot
			v.state = StateSending
			slog.Info("[Vehicle] Joined successfully",
				"vehicle", v.ID, "state", v.state, "slot", slot)
			go v.performTDMA(ctx, b, localCycleStart)
		} else {
			// Chưa thấy tên mình, gói Hello có thể bị mất. Gửi lại Hello ở cuối chu kỳ này
			slog.Warn("[Vehicle] Waiting for slot assignment...",
				"vehicle", v.ID, "state", v.state)

			// sleep to send HELLO (retry)
			v.sleepUntil(ctx, registerTime)
			if v.assignedSlot != -1 {
				v.state = StateSending
				slog.Info("[Vehicle] Slot assigned during wait => Skip resend",
					"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot)
				// start TDMA for this cycle if possible
				go v.performTDMA(ctx, b, localCycleStart)
				return
			}

			msg := model.HelloMessage{
				Type:      model.PacketHello,
				VehicleID: v.ID,
			}
			payload, _ := cbor.Marshal(msg)
			if err := v.lora.WriteLine(payload); err != nil {
				slog.Warn("[Vehicle] Failed to write HELLO (retry)",
					"vehicle", v.ID, "state", v.state, "err", err)
			} else {
				slog.Info("[Vehicle] Sent HELLO (Retry)",
					"vehicle", v.ID, "state", v.state)
			}
			// State vẫn là Joining
		}

	case StateSending:
		// Trạng thái GỬI: Kiểm tra lại SlotMap xem còn được cấp phép không
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot // Cập nhật slot nếu Gateway thay đổi
			go v.performTDMA(ctx, b, localCycleStart)
		} else {
			v.state = StateIdle
			v.assignedSlot = -1
			slog.Warn("[Vehicle] Lost slot allocation",
				"vehicle", v.ID, "state", v.state)
		}
	}
}

// performTDMA now accepts ctx and uses sleepUntilSlot to schedule transmission
func (v *Vehicle) performTDMA(ctx context.Context, b model.BeaconMessage, localCycleStart time.Time) {
	if v.assignedSlot < 1 {
		slog.Warn("[Vehicle] Invalid slot => Skipping TDMA",
			"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot)
		return
	}

	slotIndex := v.assignedSlot - 1
	slotStart := localCycleStart.Add(5*time.Millisecond +
		time.Duration(
			b.BeaconWindowMs+
				b.ControlWindowMs+
				b.GuardTimeMs+
				b.RegisterWindowMs+
				b.GuardTimeMs+
				(b.SlotWindowMs+b.GuardTimeMs)*int64(slotIndex))*time.Millisecond)
	v.sleepUntil(ctx, slotStart)

	// Re-check assigned slot hasn't changed
	if v.assignedSlot-1 != slotIndex {
		slog.Info("[Vehicle] Assigned slot changed before transmit => Skipping",
			"vehicle", v.ID, "state", v.state,
			"slotIndex", slotIndex,
			"currentSlot", v.assignedSlot,
		)
		return
	}

	// Lấy dữ liệu mới nhất
	v.mutexTelemetry.Lock()
	data := v.lastTelemetry
	v.mutexTelemetry.Unlock()

	// Đóng gói
	pkt := model.VehicleData{
		Type:        model.PacketTelemetry,
		VehicleID:   v.ID,
		Latitude:    data.Latitude,
		Longitude:   data.Longitude,
		CurrentHead: data.CurrentHead,
		TargetHead:  data.TargetHead,
		LeftSpeed:   data.LeftSpeed,
		RightSpeed:  data.RightSpeed,
	}
	payload, _ := cbor.Marshal(pkt)
	if err := v.lora.WriteLine(payload); err != nil {
		slog.Warn("[Vehicle] Failed to send TELEMETRY", "vehicle", v.ID, "state", v.state, "err", err)
		return
	}
	slog.Info("[Vehicle] Sent TELEMETRY (TDMA)",
		"vehicle", v.ID,
		"state", v.state,
		"slot", v.assignedSlot,
		"lat", data.Latitude,
		"lon", data.Longitude,
	)
}

// Hàm phụ trợ giúp sleep chính xác và hỗ trợ cancel context
func (v *Vehicle) sleepUntil(ctx context.Context, target time.Time) {
	d := time.Until(target)
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}
