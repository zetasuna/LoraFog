// FINAL

#include <Arduino.h>
#include <QMC5883LCompass.h>
#include <SPI.h>
#include <Servo.h>
#include <SoftwareSerial.h>
#include <TinyGPSPlus.h>
// #include <nRF24L01.h>
// #include <RF24.h>

#define RUDDER_PIN 6
#define THROTTLE_PIN 7
#define GPS_TX_PIN 3
#define GPS_RX_PIN 4
// #define CE_PIN 9
// #define CSN_PIN 10
//
// const byte my_address[5] = {'R', 'x', 'A', 'A', 'A'};
//
// RF24 radio(CE_PIN, CSN_PIN);
QMC5883LCompass compass;
Servo right_esc;
Servo left_esc;
SoftwareSerial gpsSerial(GPS_TX_PIN, GPS_RX_PIN);
TinyGPSPlus gps;

struct control_data {
  int16_t start;           // 0: Off, 1: Manual, 2: Auto
  int16_t max_speed;       // ppm
  int16_t cruise_speed;    // ppm
  int16_t desired_heading; // degrees
  float kp;
  float ki;
  float kd;
  float target_lat; // degrees
  float target_lon; // degrees
};

struct telemetry_data {
  int16_t throttle;          // ppm
  int16_t rudder;            // ppm
  int16_t left_motor_speed;  // ppm
  int16_t right_motor_speed; // ppm
  int16_t current_heading;   // degrees
  float lat;                 // degrees
  float lon;                 // degrees
};

struct GPSLocation {
  float lat;
  float lon;
};

struct control_data c_data;
struct telemetry_data t_data;

bool new_data = false;
int16_t left_motor_speed = 1000;
int16_t right_motor_speed = 1000;
int16_t last_left_speed = 1000;  // Biến lưu giá trị trước đó để so sánh
int16_t last_right_speed = 1000; // Biến lưu giá trị trước đó để so sánh

unsigned long last_gps_update = 0;
const unsigned long gps_update_interval = 1000;
unsigned long last_telemetry_update = 0;
const unsigned long telemetry_update_interval = 100;

// === Function Prototypes ===
void get_control_data();
// void print_control_data();
void update_telemetry_data();
int16_t get_throttle();
int16_t get_rudder();
int16_t get_heading();
GPSLocation get_gps_location();
int16_t get_turn_angle(int16_t current, int16_t desired);
int16_t get_left_speed_auto(struct control_data c_data, int16_t error,
                            int16_t turn_direction);
int16_t get_right_speed_auto(struct control_data c_data, int16_t error,
                             int16_t turn_direction);

void setup() {
  Serial.begin(9600);
  gpsSerial.begin(9600);
  // Serial.println("Autonomous Surface Vehicle Telemetry Stream");

  compass.init();

  pinMode(RUDDER_PIN, INPUT);
  pinMode(THROTTLE_PIN, INPUT);

  // Serial.println("[INFO]\tArming motors");
  right_esc.attach(2);
  left_esc.attach(5);
  right_esc.writeMicroseconds(1000);
  left_esc.writeMicroseconds(1000);
  delay(2000);
  // Serial.println("[INFO]\tMotors are armed");

  t_data.throttle = 1000;
  t_data.rudder = 1500;
  t_data.left_motor_speed = 1000;
  t_data.right_motor_speed = 1000;
  t_data.current_heading = 0;
  t_data.lat = 0.0;
  t_data.lon = 0.0;
}

void loop() {
  while (gpsSerial.available() > 0) {
    char c = gpsSerial.read(); // đọc từng ký tự từ GPS

    // In ra toàn bộ chuỗi gốc (raw NMEA)
    Serial.write(c);

    // Giải mã bằng TinyGPSPlus
    gps.encode(c);
  }

  unsigned long current_millis = millis();

  // Cập nhật GPS
  if (current_millis - last_gps_update >= gps_update_interval) {
    // Serial.print("[HEARTBEAT]\tAlive at ");
    // Serial.println(current_millis);
    while (gpsSerial.available() > 0) {
      if (gps.encode(gpsSerial.read())) {
        if (gps.location.isValid()) {
          get_gps_location();
          // GPSLocation location = get_gps_location();
          // Serial.print("GPS: Lat=");
          // Serial.print(location.lat, 6);
          // Serial.print(", Lon=");
          // Serial.println(location.lon, 6);
        }
      }
    }
    last_gps_update = current_millis;
  }

  // Nhận dữ liệu điều khiển từ remote
  get_control_data();
  if (new_data == true) {
    // print_control_data();
    new_data = false;
  }

  // Điều khiển boat theo mode
  if (c_data.start == 0) {
    left_esc.writeMicroseconds(1000);
    right_esc.writeMicroseconds(1000);
    left_motor_speed = 1000;
    right_motor_speed = 1000;
  } else if (c_data.start == 1) { // Manual mode
    int16_t throttleValue = pulseIn(THROTTLE_PIN, HIGH, 25000);
    int16_t rudderValue = pulseIn(RUDDER_PIN, HIGH, 25000);

    if (throttleValue < 1000 || throttleValue > 2000 || throttleValue == 0) {
      throttleValue = 1000;
    }
    if (rudderValue < 1000 || rudderValue > 2000 || rudderValue == 0) {
      rudderValue = 1500;
    }

    int16_t rudderOffset = map(rudderValue, 1000, 2000, -500, 500);
    left_motor_speed = constrain(throttleValue - rudderOffset, 1000, 2000);
    right_motor_speed = constrain(throttleValue + rudderOffset, 1000, 2000);

    left_esc.writeMicroseconds(left_motor_speed);
    right_esc.writeMicroseconds(right_motor_speed);
  } else if (c_data.start == 2) { // Auto mode
    int16_t current_heading = get_heading();
    int16_t desired_heading = c_data.desired_heading;
    int16_t turn_angle = get_turn_angle(current_heading, desired_heading);
    int16_t turn_direction = (turn_angle > 180) ? -1 : 1;
    int16_t error = (turn_angle > 180) ? (turn_angle - 360) : turn_angle;

    left_motor_speed = get_left_speed_auto(c_data, error, turn_direction);
    right_motor_speed = get_right_speed_auto(c_data, error, turn_direction);

    left_motor_speed = constrain(left_motor_speed, 1000, 2000);
    right_motor_speed = constrain(right_motor_speed, 1000, 2000);

    left_esc.writeMicroseconds(left_motor_speed);
    right_esc.writeMicroseconds(right_motor_speed);
  }

  // Cập nhật telemetry và hiển thị motor speed khi có thay đổi
  if (current_millis - last_telemetry_update >= telemetry_update_interval) {
    update_telemetry_data();
    // if (radio.isAckPayloadAvailable()) {
    //   radio.writeAckPayload(1, &t_data, sizeof(t_data));
    // }

    // Chỉ in khi tốc độ động cơ thay đổi
    if (left_motor_speed != last_left_speed ||
        right_motor_speed != last_right_speed) {
      // Serial.print("[MOTOR]\tLeft Speed: ");
      // Serial.print(left_motor_speed);
      // Serial.print(" | Right Speed: ");
      // Serial.println(right_motor_speed);
      last_left_speed = left_motor_speed;
      last_right_speed = right_motor_speed;
    }

    last_telemetry_update = current_millis;
  }
}

void get_control_data() {
  // if (radio.available()) {
  //   radio.read(&c_data, sizeof(c_data));
  //   update_telemetry_data();
  //   radio.writeAckPayload(1, &t_data, sizeof(t_data));
  //   new_data = true;
  // }
  if ((size_t)Serial.available() >= sizeof(c_data)) {
    Serial.readBytes((byte *)&c_data, sizeof(c_data)); // Nhận raw struct
    update_telemetry_data();
    Serial.write((byte *)&t_data, sizeof(t_data)); // Gửi phản hồi
    new_data = true;
  }
}

// void print_control_data() {
//   Serial.print("[CONTROL]\t");
//   Serial.print(c_data.start);
//   Serial.print("\t");
//   Serial.print(c_data.max_speed);
//   Serial.print("\t");
//   Serial.print(c_data.cruise_speed);
//   Serial.print("\t");
//   Serial.print(c_data.desired_heading);
//   Serial.print("\t");
//   Serial.print(c_data.kp);
//   Serial.print("\t");
//   Serial.print(c_data.ki);
//   Serial.print("\t");
//   Serial.print(c_data.kd);
//   Serial.println();
// }

void update_telemetry_data() {
  t_data.throttle = get_throttle();
  t_data.rudder = get_rudder();
  t_data.left_motor_speed = left_motor_speed;
  t_data.right_motor_speed = right_motor_speed;
  t_data.current_heading = get_heading();

  GPSLocation location = get_gps_location();
  t_data.lat = location.lat;
  t_data.lon = location.lon;
}

int16_t get_throttle() {
  int16_t raw_throttle = pulseIn(THROTTLE_PIN, HIGH, 25000);
  if (raw_throttle == 0 || raw_throttle < 1000 || raw_throttle > 2000) {
    return 1000;
  }
  return raw_throttle;
}

int16_t get_rudder() {
  int16_t raw_rudder = pulseIn(RUDDER_PIN, HIGH, 25000);
  if (raw_rudder == 0 || raw_rudder < 1000 || raw_rudder > 2000) {
    return 1500;
  }
  return raw_rudder;
}

int16_t get_heading() {
  compass.read();
  int16_t azimuth = compass.getAzimuth();
  if (azimuth < 0)
    azimuth += 360;
  return azimuth;
}

GPSLocation get_gps_location() {
  GPSLocation location;
  location.lat = gps.location.isValid() ? gps.location.lat() : 0.0;
  location.lon = gps.location.isValid() ? gps.location.lng() : 0.0;
  return location;
}

int16_t get_turn_angle(int16_t current, int16_t desired) {
  return (desired - current + 360) % 360;
}

int16_t get_left_speed_auto(struct control_data c_data, int16_t error,
                            int16_t turn_direction) {
  int16_t left_speed =
      c_data.cruise_speed + (c_data.kp * error * turn_direction);
  if (left_speed < 1000)
    left_speed = 1000;
  if (left_speed > c_data.max_speed)
    left_speed = c_data.max_speed;
  return left_speed;
}

int16_t get_right_speed_auto(struct control_data c_data, int16_t error,
                             int16_t turn_direction) {
  int16_t right_speed =
      c_data.cruise_speed - (c_data.kp * error * turn_direction);
  if (right_speed < 1000)
    right_speed = 1000;
  if (right_speed > c_data.max_speed)
    right_speed = c_data.max_speed;
  return right_speed;
}
