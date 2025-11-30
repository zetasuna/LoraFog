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
		estOWDms:     150, // default initial estimate (ms)
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
			Latitude:    21.0532 + rand.Float64()*0.001,
			Longitude:   105.8261 + rand.Float64()*0.001,
			CurrentHead: 0,    // + rand.Int63n(361),
			TargetHead:  0,    // + rand.Int63n(361),
			LeftSpeed:   1000, // + rand.Int63n(1000),
			RightSpeed:  1000, // + rand.Int63n(1000),
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
				arduinoControl := fmt.Sprintf("%d,%.6f,%.6f,%.6f,%.6f,%.6f",
					control.Speed,
					control.Latitude,
					control.Longitude,
					control.Kp,
					control.Ki,
					control.Kd,
				)
				// Forward control data to Arduino
				if err := v.arduino.Write(arduinoControl); err != nil {
					slog.Error("Failed to forward control to Arduino", "vehicle", v.ID, "error", err)
				} else {
					slog.Info("Forwarded control to Arduino", "vehicle", v.ID)
				}
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

		// use sleepUntilRegisterWindow to compensate timing
		if ok := v.sleepUntilRegisterWindow(ctx, b); !ok {
			return
		}

		// Re-check: maybe beacon or another goroutine assigned slot
		if v.assignedSlot != -1 {
			// already have slot -> go to sending
			v.state = StateSending
			slog.Info("Already assigned slot while waiting, state -> SENDING", "vehicle", v.ID, "slot", v.assignedSlot)
			return
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
			go v.performTDMA(ctx, b)
		} else {
			// Chưa thấy tên mình, gói Hello có thể bị mất. Gửi lại Hello ở cuối chu kỳ này
			slog.Warn("Waiting for slot assignment...", "id", v.ID)

			// use sleepUntilRegisterWindow for retry
			if ok := v.sleepUntilRegisterWindow(ctx, b); !ok {
				return
			}

			// Re-check prior to sending
			if v.assignedSlot != -1 {
				slog.Info("Slot assigned during wait, skip resend", "vehicle", v.ID, "slot", v.assignedSlot)
				v.state = StateSending
				// start TDMA for this cycle if possible
				go v.performTDMA(ctx, b)
				return
			}

			msg := model.HelloMessage{Type: model.PacketHello, VehicleID: v.ID}
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
			go v.performTDMA(ctx, b)
		} else {
			slog.Warn("Lost slot allocation, returning to IDLE", "id", v.ID)
			v.state = StateIdle
			v.assignedSlot = -1
		}
	}
}

// performTDMA now accepts ctx and uses sleepUntilSlot to schedule transmission
func (v *Vehicle) performTDMA(ctx context.Context, b model.BeaconMessage) {
	if v.assignedSlot < 1 {
		slog.Warn("Invalid slot, skipping TDMA", "vehicle", v.ID, "slot", v.assignedSlot)
		return
	}

	// compute 0-based index
	slotIndex := v.assignedSlot - 1

	// Wait until the slot (potentially current or next cycle)
	if ok := v.sleepUntilSlot(ctx, b, slotIndex); !ok {
		// skip if context canceled or timing not possible
		return
	}

	// Re-check assigned slot hasn't changed
	if v.assignedSlot-1 != slotIndex {
		slog.Info("Assigned slot changed before transmit, skipping", "vehicle", v.ID, "slotIndex", slotIndex, "currentSlot", v.assignedSlot)
		return
	}

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

	// Gửi và đo RTT to update estOWD
	start := time.Now()
	if err := v.lora.Write(payload); err != nil {
		slog.Warn("Failed to send telemetry", "err", err)
		return
	}
	// // we don't have ack mechanism here; as approximation, measure local write duration
	// elapsed := time.Since(start)
	// // update estOWD as EWMA of previous and elapsed/2
	// v.owdMu.Lock()
	// prev := v.estOWDms
	// meas := int64(elapsed.Milliseconds() / 2)
	// alpha := 0.7
	// v.estOWDms = int64(math.Round(alpha*float64(prev) + (1.0-alpha)*float64(meas)))
	// v.owdMu.Unlock()

	// we cannot measure true OWD without ACK; use conservative EWMA & bounds
	elapsed := time.Since(start)
	meas := int64(elapsed.Milliseconds() / 2)
	if meas < 10 {
		meas = 10
	}
	if meas > 2000 {
		meas = 2000
	}
	// alpha smaller to reduce oscillation
	alpha := 0.3
	v.owdMu.Lock()
	prev := v.estOWDms
	v.estOWDms = int64(math.Round(alpha*float64(meas) + (1.0-alpha)*float64(prev)))
	v.owdMu.Unlock()

	slog.Debug("Sent Telemetry (TDMA)",
		"vehicle", v.ID,
		"slot", v.assignedSlot,
		"lat", data.Latitude,
		"lon", data.Longitude,
		"estOWDms", v.estOWDms,
	)
}

// sleepUntilRegisterWindow sleeps until the appropriate time to send HELLO
// It uses NextCycleStart and regStartOffset logic and compensates for estimated one-way delay
func (v *Vehicle) sleepUntilRegisterWindow(ctx context.Context, b model.BeaconMessage) bool {
	cycleMs := int64(b.CycleDurationMs)
	regWindowMs := int64(b.RegisterWindowMs)
	regStartOffset := cycleMs - regWindowMs

	// compute register window start for the next cycle
	regStartMs := b.NextCycleStart + regStartOffset

	// estimate one-way delay
	v.owdMu.Lock()
	estOWD := v.estOWDms
	v.owdMu.Unlock()

	// safety margin and minimal prep time
	const safetyMs = int64(60)
	const minPrepMs = int64(40)

	nowMs := time.Now().UnixNano() / int64(time.Millisecond)

	// target time to start transmitting so arrival inside window
	targetTxMs := regStartMs - estOWD - safetyMs
	sleepMs := targetTxMs - nowMs

	if sleepMs <= 0 {
		// maybe still possible to send (small backoff) if we are within window
		regEndMs := regStartMs + regWindowMs
		arrivalIfNow := nowMs + estOWD
		// if arrival before end and we have time to prepare -> do short random backoff
		if arrivalIfNow+minPrepMs < regEndMs {
			backoff := rand.Int63n(int64(math.Min(float64(regWindowMs/4), 200)))
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Duration(backoff) * time.Millisecond):
				return true
			}
		}
		// else skip to next cycle
		nextRegStart := regStartMs + cycleMs
		sleepMs = nextRegStart - nowMs
	}

	if sleepMs > 0 {
		timer := time.NewTimer(time.Duration(sleepMs) * time.Millisecond)
		// defer timer.Stop()
		defer func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			// IMPORTANT: re-check state/assignedSlot/last received beacon timestamp
			if v.state == StateJoining && v.assignedSlot != -1 {
				// slot already assigned while sleeping -> do NOT resend
				return true
			}
			return true
		}
	}
	return true
}

// sleepUntilSlot sleeps until the correct slot start (supports current or next cycle)
func (v *Vehicle) sleepUntilSlot(ctx context.Context, b model.BeaconMessage, slotIndex int) bool {
	// slotIndex assumed 0-based
	slotDurMs := int64(b.SlotDurationMs)
	guardMs := int64(b.GuardTimeMs)
	oneSlotMs := slotDurMs + guardMs
	// cycleMs := int64(b.CycleDurationMs)

	// compute candidate starts: current cycle and next cycle
	candidates := []int64{b.CycleStart, b.NextCycleStart}

	v.owdMu.Lock()
	estOWD := v.estOWDms
	v.owdMu.Unlock()

	const safetyMs = int64(40)
	const minPrepMs = int64(20)

	nowMs := time.Now().UnixNano() / int64(time.Millisecond)

	for _, base := range candidates {
		slotStart := base + guardMs + int64(slotIndex)*oneSlotMs
		// target transmit so arrival at slotStart
		targetTxMs := slotStart - estOWD - safetyMs
		sleepMs := targetTxMs - nowMs
		if sleepMs <= 0 {
			// maybe still possible if arrival before slot end
			slotEnd := slotStart + slotDurMs
			arrivalIfNow := nowMs + estOWD
			if arrivalIfNow+minPrepMs < slotEnd {
				// tiny random backoff to avoid collisions
				backoff := rand.Int63n(50)
				select {
				case <-ctx.Done():
					return false
				case <-time.After(time.Duration(backoff) * time.Millisecond):
					return true
				}
			}
			// else try next candidate
			continue
		}
		// wait until targetTxMs
		timer := time.NewTimer(time.Duration(sleepMs) * time.Millisecond)
		// defer timer.Stop()
		defer func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			// IMPORTANT: re-check state/assignedSlot/last received beacon timestamp
			if v.state == StateJoining && v.assignedSlot != -1 {
				// slot already assigned while sleeping -> do NOT resend
				return false
			}
			return true
		}
	}
	// if nothing matched, wait a short time then return false to let caller skip
	select {
	case <-ctx.Done():
		return false
	case <-time.After(100 * time.Millisecond):
		return false
	}
}
