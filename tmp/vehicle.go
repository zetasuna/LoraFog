// Package core implements the Vehicle agent
// responsible for collecting telemetry from Arduino devices
// and sending CBOR-encoded data via LoRa.
package core

import (
	"context"
	"log/slog"
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
	mu            sync.Mutex
	lastTelemetry model.ArduinoData

	// estimated one way delay (ms) for timing compensation
	estOWDms int64
	owdMu    sync.Mutex

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
		estOWDms:     150,
	}
	if arduinoDev != "" {
		v.arduino = device.NewArduino(arduinoDev, arduinoBaud)
	}
	return v
}

func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	slog.Info("Vehicle started", "id", v.ID)

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
			slog.Warn("Failed to close LoRa device", "error", err)
		}
	}
	if v.arduino != nil {
		if err := v.arduino.Close(); err != nil {
			slog.Warn("Failed to close Arduino device", "error", err)
		}
	}
	v.wg.Wait()
	slog.Info("Vehicle stopped", "id", v.ID)
}

// arduinoLoop đọc dữ liệu từ Arduino/Simulator
func (v *Vehicle) arduinoLoop(ctx context.Context) {
	defer v.wg.Done()
	dataCh := make(chan model.ArduinoData, 5)
	stop, err := v.arduino.Read(dataCh)
	if err != nil {
		slog.Warn("Arduino.Read returned error", "err", err)
		return
	}
	defer stop()

	// Khởi tạo data giả nếu arduino nil
	if v.arduino == nil {
		v.mu.Lock()
		v.lastTelemetry = model.ArduinoData{
			Latitude:    21.027 + rand.Float64()*0.001,
			Longitude:   105.835 + rand.Float64()*0.001,
			CurrentHead: rand.Int63n(361),
			TargetHead:  rand.Int63n(361),
			LeftSpeed:   1000 + rand.Int63n(1000),
			RightSpeed:  1000 + rand.Int63n(1000),
		}
		v.mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dataCh:
			if !ok {
				return
			}
			v.mu.Lock()
			v.lastTelemetry = data
			v.mu.Unlock()
			slog.Info("Vehicle received arduino data",
				"vehicle", v.ID,
				"lat", data.Latitude,
				"lon", data.Longitude,
			)
		}
	}
}

// loraLoop là State Machine chính của Vehicle
func (v *Vehicle) loraLoop(ctx context.Context) {
	defer v.wg.Done()

	// Sử dụng giá trị mặc định cho timeout chờ beacon (ví dụ: 5 giây)
	beaconTimeout := 5 * time.Second
	var lastBeaconTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Lắng nghe Beacon
			frame, err := v.lora.Read(beaconTimeout)
			if err != nil {
				// Nếu timeout hoặc lỗi sau khi đã từng có session -> Reset về IDLE
				// timeout handling
				if err == device.ErrLoraTimeout {
					// if we had a previous beacon and too long passed -> reset
					if !lastBeaconTime.IsZero() && time.Since(lastBeaconTime) > 2*beaconTimeout {
						if v.state != StateIdle {
							slog.Warn("Lost beacon connection, resetting to IDLE", "id", v.ID)
						}
						v.state = StateIdle
						v.currentGateway = ""
						v.assignedSlot = -1
					}
					continue
				}
				// Nếu đã Idle thì cứ tiếp tục lắng nghe
				slog.Error("Lora read error", "err", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Phân loại gói tin
			var generic map[string]any
			if err := cbor.Unmarshal(frame, &generic); err != nil {
				slog.Warn("Received unknown or corrupted CBOR packet", "err", err)
				continue
			}

			msgType, ok := generic["type"].(string)
			if !ok {
				slog.Warn("Packet type missing")
				continue
			}

			switch msgType {
			case model.PacketBeacon:
				var beacon model.BeaconMessage
				if err := cbor.Unmarshal(frame, &beacon); err != nil {
					// Có thể là packet Control hoặc nhiễu
					slog.Debug("Received non-beacon frame or corrupted beacon", "err", err)
					continue
				}
				slog.Info("Received beacon", "vehicle", v.ID)
				// record last beacon time and timestamp (ms)
				lastBeaconTime = time.Now()
				// Xử lý logic dựa trên State và Beacon nhận được
				v.handleBeacon(ctx, beacon)
			case model.PacketControl:
				var control model.ControlData
				if err := cbor.Unmarshal(frame, &control); err != nil {
					// Có thể là packet Control hoặc nhiễu
					slog.Debug("Received non-control frame or corrupted control", "err", err)
					continue
				}
				slog.Info("Received control", "vehicle", v.ID)
			default:
				slog.Debug("Received unhandled message type", "type", msgType)
			}
		}
	}
}

func (v *Vehicle) handleBeacon(ctx context.Context, b model.BeaconMessage) {
	// Logic Roaming: Nếu gateway ID khác với hiện tại và đã có slot
	if v.state == StateSending && v.currentGateway != "" && v.currentGateway != b.GatewayAddress {
		slog.Info("Roaming detected, preparing to switch", "old", v.currentGateway, "new", b.GatewayAddress)
		v.state = StateIdle // Reset về Idle để đăng ký lại với Gateway mới
		v.assignedSlot = -1
		// gửi lại hello ngay lập tức
		hello := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(hello)
		if err := v.lora.Write(payload); err != nil {
			slog.Warn("Failed to send hello during roaming", "err", err)
		} else {
			slog.Info("Sent HELLO after roaming", "vehicle", v.ID)
			v.state = StateJoining
		}
		v.currentGateway = b.GatewayAddress
		return
	}
	// Luôn cập nhật Gateway ID mới nhất
	v.currentGateway = b.GatewayAddress

	switch v.state {
	case StateIdle:
		slog.Info("Received Beacon (Idle), preparing to send Hello", "gw", b.GatewayAddress)

		// Tính thời gian đợi đến Register Window
		cycleDuration := time.Duration(b.CycleDurationMs) * time.Millisecond
		regWindow := time.Duration(b.RegisterWindowMs) * time.Millisecond
		regStartOffset := cycleDuration - regWindow

		// Thời gian ngủ: Bằng độ dài chu kỳ - thời điểm bắt đầu Register Window + delay ngẫu nhiên nhỏ
		// Ví dụ: Chu kỳ 2000ms, Register Window 300ms. regStartOffset = 1700ms.
		// Time to sleep = 1700ms + (0-150ms)
		// timeToSleep := regStartOffset + time.Duration(rand.Int63n(b.RegisterWindowMs/2))*time.Millisecond
		// time.Sleep(timeToSleep)
		// Calculate actual offset between local time and beacon timestamp (ms)
		now := time.Now().UnixNano() / int64(time.Millisecond)
		delta := now - b.CycleStart
		// time until regStart since now = regStartOffset - delta
		sleep := regStartOffset - time.Duration(delta)*time.Millisecond
		if sleep < 0 {
			// if we are already in/after reg window, send soon (small random backoff)
			sleep = time.Duration(rand.Int63n(int64(regWindow / 4)))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleep):
		}

		// Gửi Hello
		msg := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(msg)
		if err := v.lora.Write(payload); err != nil {
			slog.Warn("Failed to write Hello", "err", err)
		} else {
			v.state = StateJoining
			slog.Info("Sent Hello, state -> JOINING", "id", v.ID)
		}
	case StateJoining:
		// Trạng thái CHUẨN BỊ GỬI: Kiểm tra xem trong Beacon mới có Slot cho mình chưa
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot
			v.state = StateSending
			slog.Info("Joined successfully state -> SENDING", "vehicle", v.ID, "slot", slot)
			// Sau khi nhận beacon (T0), thực hiện TDMA gửi ngay trong chu kỳ này
			v.performTDMA(b)
		} else {
			// Chưa thấy tên mình, gói Hello có thể bị mất. Gửi lại Hello ở cuối chu kỳ này
			slog.Warn("Waiting for slot assignment...", "id", v.ID)

			cycleDuration := time.Duration(b.CycleDurationMs) * time.Millisecond
			regWindow := time.Duration(b.RegisterWindowMs) * time.Millisecond
			regStartOffset := cycleDuration - regWindow
			// regStartOffset := cycleDuration - time.Duration(b.RegisterWindowMs)*time.Millisecond

			// Ngủ đến Register Window tiếp theo
			// time.Sleep(regStartOffset + time.Duration(rand.Int63n(b.RegisterWindowMs/2))*time.Millisecond)

			now := time.Now().UnixNano() / int64(time.Millisecond)
			delta := now - b.CycleStart
			sleep := regStartOffset - time.Duration(delta)*time.Millisecond
			if sleep < 0 {
				sleep = time.Duration(rand.Int63n(int64(regWindow / 4)))
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleep):
			}

			msg := model.HelloMessage{Type: "hello", VehicleID: v.ID}
			payload, _ := cbor.Marshal(msg)
			if err := v.lora.Write(payload); err != nil {
				slog.Warn("Failed to write Hello (retry)", "err", err)
			} else {
				slog.Info("Resent Hello (JOINING)", "vehicle", v.ID)
			}
			// State vẫn là Joining
		}

	case StateSending:
		// Trạng thái GỬI: Kiểm tra lại SlotMap xem còn được cấp phép không
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot // Cập nhật slot nếu Gateway thay đổi
			v.performTDMA(b)
		} else {
			slog.Warn("Lost slot allocation, returning to IDLE", "id", v.ID)
			v.state = StateIdle
			v.assignedSlot = -1
		}
	}
}

// performTDMA tính toán thời gian ngủ và gửi Telemetry đúng Slot
func (v *Vehicle) performTDMA(b model.BeaconMessage) {
	// Tính toán thời điểm gửi: (SlotIndex-1) * (SlotDur + Guard)
	// SlotIndex 1: (1-1)*... = 0ms. Gửi ngay. (Đây là cách tính đơn giản)
	// SlotIndex n: (n-1) * (SlotDur + Guard)

	slotIndex := max(0, v.assignedSlot-1)
	oneSlot := time.Duration(b.SlotDurationMs)*time.Millisecond + time.Duration(b.GuardTimeMs)*time.Millisecond
	slotTimeFromStart := time.Duration(slotIndex) * oneSlot

	// Calculate delta between now and beacon timestamp
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)
	deltaMs := nowMs - b.CycleStart
	sleep := slotTimeFromStart - time.Duration(deltaMs)*time.Millisecond
	if sleep < 0 {
		// if we missed slot, do not block; wait next cycle
		slog.Debug(
			"Missed slot timing, skipping this cycle",
			"vehicle", v.ID, "slot", v.assignedSlot)
		return
	}

	// Ngủ đến đúng slot
	// time.Sleep(slotTimeFromStart)
	time.Sleep(sleep)

	// Lấy dữ liệu mới nhất
	v.mu.Lock()
	data := v.lastTelemetry
	v.mu.Unlock()

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

	// Gửi
	if err := v.lora.Write(payload); err != nil {
		slog.Warn("Failed to send telemetry", "err", err)
	} else {
		slog.Debug("Sent Telemetry (TDMA)",
			"vehicle", v.ID,
			"slot", v.assignedSlot,
			"lat", data.Latitude,
			"lon", data.Longitude,
		)
	}
}
