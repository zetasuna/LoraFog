// FINAL

#include <Arduino.h>
#include <QMC5883LCompass.h>
#include <Servo.h>
#include <SoftwareSerial.h>
#include <TinyGPSPlus.h>
#include <stdint.h>

// === CONFIG ===
#define GPS_TX_PIN 3
#define GPS_RX_PIN 4
#define RIGHT_ESC_PIN 2
#define LEFT_ESC_PIN 5
#define SERIAL_BAUD 9600
#define GPS_BAUD 9600
#define MIN_PPM 1000
#define MAX_PPM 2000
#define DEFAULT_LATITUDE 0.0
#define DEFAULT_LONGITUDE 0.0
#define DEFAULT_HEADING 0
#define UPDATE_INTERVAL 1000
#define DISTANCE_STOP 2.0 // 2m

// === STRUCT ===
struct TelemetryData {
  float latitude;
  float longitude;
  int16_t left_motor_speed;  // ppm
  int16_t right_motor_speed; // ppm
  int16_t current_heading;   // degrees
  int16_t desired_heading;   // degrees
};

struct ControlData {
  int16_t cruise_speed; // ppm
  float latitude;
  float longitude;
  float kp;
  float ki;
  float kd;
};

// === GLOBALS ===
QMC5883LCompass compass;
Servo right_esc;
Servo left_esc;
SoftwareSerial gpsSerial(GPS_TX_PIN, GPS_RX_PIN);
TinyGPSPlus gps;

struct TelemetryData t_data;
struct ControlData c_data;

bool has_target = false;
int16_t left_motor_speed = MIN_PPM;
int16_t right_motor_speed = MIN_PPM;
float latitude = DEFAULT_LATITUDE;
float longitude = DEFAULT_LONGITUDE;
int16_t current_heading = DEFAULT_HEADING;
int16_t desired_heading = DEFAULT_HEADING;
unsigned long last_update = 0;

// === FUNCTION DECLARATIONS ===
void autoControl();
void stopBoat();
bool isAtTarget(float current_latitude, float current_longitude,
                float target_latitude, float target_longitude);
void updateTelemetry();
void sendTelemetry();
void processControlMessage(const String &line);
int16_t getHeading();
int16_t calculateBearing(float current_latitude, float current_longitude,
                         float target_latitude, float target_longitude);
int16_t calculateTurnAngle(int16_t current, int16_t desired);
int16_t calculateLeftSpeed(int16_t error, int16_t turn_direction);
int16_t calculateRightSpeed(int16_t error, int16_t turn_direction);

// === SETUP ===
void setup() {
  Serial.begin(SERIAL_BAUD);
  gpsSerial.begin(GPS_BAUD);
  compass.init();

  right_esc.attach(RIGHT_ESC_PIN);
  left_esc.attach(LEFT_ESC_PIN);
  right_esc.writeMicroseconds(MIN_PPM);
  left_esc.writeMicroseconds(MIN_PPM);
  delay(2000);

  t_data.latitude = DEFAULT_LATITUDE;
  t_data.longitude = DEFAULT_LONGITUDE;
  t_data.left_motor_speed = MIN_PPM;
  t_data.right_motor_speed = MIN_PPM;
  t_data.current_heading = DEFAULT_HEADING;
  t_data.desired_heading = DEFAULT_HEADING;

  stopBoat();
}

// === LOOP ===
void loop() {
  unsigned long now = millis();
  if (now - last_update >= UPDATE_INTERVAL) {
    autoControl();
    updateTelemetry();
    sendTelemetry();
    last_update = now;
  }

  if (gpsSerial.available()) {
    if (gps.encode(gpsSerial.read())) {
      latitude = gps.location.isValid() ? gps.location.lat() : 0.0;
      longitude = gps.location.isValid() ? gps.location.lng() : 0.0;
      if (has_target && gps.location.isValid()) {
        desired_heading = calculateBearing(latitude, longitude, c_data.latitude,
                                           c_data.longitude);
      }
    }
  }

  if (Serial.available()) {
    String control = Serial.readStringUntil('\n');
    control.trim();
    if (control.length() > 0) {
      processControlMessage(control);
      has_target = true;
    }
  }
}

// === CONTROL BOAT ===
void autoControl() {
  if (has_target &&
      isAtTarget(latitude, longitude, c_data.latitude, c_data.longitude)) {
    has_target = false;
    stopBoat();
    return;
  }
  current_heading = getHeading();
  int16_t turn_angle = calculateTurnAngle(current_heading, desired_heading);
  int16_t turn_direction = (turn_angle > 180) ? -1 : 1;
  int16_t error = (turn_angle > 180) ? (turn_angle - 360) : turn_angle;
  error = constrain(error, -90, 90);

  left_motor_speed = calculateLeftSpeed(error, turn_direction);
  right_motor_speed = calculateRightSpeed(error, turn_direction);

  left_motor_speed = constrain(left_motor_speed, MIN_PPM, MAX_PPM);
  right_motor_speed = constrain(right_motor_speed, MIN_PPM, MAX_PPM);

  left_esc.writeMicroseconds(left_motor_speed);
  right_esc.writeMicroseconds(right_motor_speed);
}

void stopBoat() {
  left_motor_speed = MIN_PPM;
  right_motor_speed = MIN_PPM;
  left_esc.writeMicroseconds(left_motor_speed);
  right_esc.writeMicroseconds(right_motor_speed);
}

// === CHECK ARRIVAL ===
bool isAtTarget(float current_latitude, float current_longitude,
                float target_latitude, float target_longitude) {
  const float EARTH_RADIUS = 6371000.0; // m
  float delta_latitude = radians(target_latitude - current_latitude);
  float delta_longitude = radians(target_longitude - current_longitude);
  float a = sin(delta_latitude / 2) * sin(delta_latitude / 2) +
            cos(radians(current_latitude)) * cos(radians(target_latitude)) *
                sin(delta_longitude / 2) * sin(delta_longitude / 2);
  float c = 2 * atan2(sqrt(a), sqrt(1 - a));
  float distance = EARTH_RADIUS * c;
  return distance < DISTANCE_STOP; // đã đến nếu cách mục tiêu < 2m
}

// === TELEMETRY ===
void updateTelemetry() {
  t_data.latitude = latitude;
  t_data.longitude = longitude;
  t_data.left_motor_speed = left_motor_speed;
  t_data.right_motor_speed = right_motor_speed;
  t_data.current_heading = current_heading;
  t_data.desired_heading = desired_heading;
}

void sendTelemetry() {
  Serial.print(t_data.latitude, 6);
  Serial.print(",");
  Serial.print(t_data.longitude, 6);
  Serial.print(",");
  Serial.print(t_data.left_motor_speed);
  Serial.print(",");
  Serial.print(t_data.right_motor_speed);
  Serial.print(",");
  Serial.print(t_data.current_heading);
  Serial.print(",");
  Serial.println(t_data.desired_heading);
}

// === CONTROL ===
void processControlMessage(const String &line) {
  int16_t last_index = 0;
  int16_t token_index = 0;
  String tokens[6];
  for (int16_t i = 0; i < 6; i++)
    tokens[i] = "";

  while (token_index < 6) {
    int comma_index = line.indexOf(',', last_index);
    if (comma_index == -1) {
      tokens[token_index++] = line.substring(last_index);
      break;
    }
    tokens[token_index++] = line.substring(last_index, comma_index);
    last_index = comma_index + 1;
  }

  c_data.cruise_speed = tokens[0].toInt();
  c_data.latitude = tokens[1].toFloat();
  c_data.longitude = tokens[2].toFloat();
  c_data.kp = tokens[3].toFloat();
  c_data.ki = tokens[4].toFloat();
  c_data.kd = tokens[5].toFloat();
  // Serial.print(c_data.kp, 6);
  // Serial.print(",");
  // Serial.print(c_data.ki, 6);
  // Serial.print(",");
  // Serial.print(c_data.kd, 6);
  // Serial.print(",");
  // Serial.print(c_data.cruise_speed);
  // Serial.print(",");
  // Serial.println(c_data.desired_heading);
}

// === UTILS ===
int16_t getHeading() {
  compass.read();
  int16_t azimuth = compass.getAzimuth();
  if (azimuth < 0)
    azimuth += 360;
  return azimuth;
}

int16_t calculateBearing(float current_latitude, float current_longitude,
                         float target_latitude, float target_longitude) {
  float delta_longitude = radians(target_longitude - current_longitude);
  float y = sin(delta_longitude) * cos(radians(target_latitude));
  float x = cos(radians(current_latitude)) * sin(radians(target_latitude)) -
            sin(radians(current_latitude)) * cos(radians(target_latitude)) *
                cos(delta_longitude);
  float bearing = atan2(y, x) * 180 / PI;
  bearing = fmodf((bearing + 360), 360);
  return int16_t(round(bearing));
}

int16_t calculateTurnAngle(int16_t current, int16_t desired) {
  return (desired - current + 360) % 360;
}

int16_t calculateLeftSpeed(int16_t error, int16_t turn_direction) {
  int16_t left_speed =
      c_data.cruise_speed + (c_data.kp * error * turn_direction);
  left_speed = constrain(left_speed, MIN_PPM, MAX_PPM);
  return left_speed;
}

int16_t calculateRightSpeed(int16_t error, int16_t turn_direction) {
  int16_t right_speed =
      c_data.cruise_speed - (c_data.kp * error * turn_direction);
  right_speed = constrain(right_speed, MIN_PPM, MAX_PPM);
  return right_speed;
}
