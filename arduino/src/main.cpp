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
#define UPDATE_INTERVAL 1000
#define DEFAULT_LATITUDE 0.0
#define DEFAULT_LONGITUDE 0.0

// === STRUCT ===
struct TelemetryData {
  int16_t left_motor_speed;  // ppm
  int16_t right_motor_speed; // ppm
  int16_t current_heading;   // degrees
  float latitude;
  float longitude;
};

struct ControlData {
  int16_t max_speed;       // ppm
  int16_t cruise_speed;    // ppm
  int16_t desired_heading; // degrees
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

int16_t left_motor_speed = MIN_PPM;
int16_t right_motor_speed = MIN_PPM;
float latitude = DEFAULT_LATITUDE;
float longitude = DEFAULT_LONGITUDE;
unsigned long last_update = 0;

// === FUNCTION DECLARATIONS ===
void updateTelemetry();
void sendTelemetry();
void processControlMessage(const String &line);
void applyControlMessage();
int16_t getHeading();
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
  t_data.current_heading = 0;
}

// === LOOP ===
void loop() {
  unsigned long now = millis();
  if (now - last_update >= UPDATE_INTERVAL) {
    updateTelemetry();
    sendTelemetry();
    last_update = now;
  }

  if (gpsSerial.available()) {
    if (gps.encode(gpsSerial.read())) {
      latitude = gps.location.isValid() ? gps.location.lat() : 0.0;
      longitude = gps.location.isValid() ? gps.location.lng() : 0.0;
    }
  }

  if (Serial.available()) {
    String control = Serial.readStringUntil('\n');
    control.trim();
    if (control.length() > 0) {
      processControlMessage(control);
      applyControlMessage();
    }
  }
}

// === TELEMETRY ===
void updateTelemetry() {
  t_data.latitude = latitude;
  t_data.longitude = longitude;
  t_data.left_motor_speed = left_motor_speed;
  t_data.right_motor_speed = right_motor_speed;
  t_data.current_heading = getHeading();
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
  Serial.println(t_data.current_heading);
}

// === CONTROL ===
void processControlMessage(const String &line) {
  int lastIndex = 0, tokenIndex = 0;
  String tokens[6];
  for (int i = 0; i < 6; i++)
    tokens[i] = "";

  while (tokenIndex < 6) {
    int commaIndex = line.indexOf(',', lastIndex);
    if (commaIndex == -1) {
      tokens[tokenIndex++] = line.substring(lastIndex);
      break;
    }
    tokens[tokenIndex++] = line.substring(lastIndex, commaIndex);
    lastIndex = commaIndex + 1;
  }

  c_data.max_speed = tokens[0].toInt();
  c_data.cruise_speed = tokens[1].toInt();
  c_data.desired_heading = tokens[2].toInt();
  c_data.kp = tokens[3].toFloat();
  c_data.ki = tokens[4].toFloat();
  c_data.kd = tokens[5].toFloat();
}

void applyControlMessage() {
  int16_t current_heading = getHeading();
  int16_t desired_heading = c_data.desired_heading;
  int16_t turn_angle = calculateTurnAngle(current_heading, desired_heading);
  int16_t turn_direction = (turn_angle > 180) ? -1 : 1;
  int16_t error = (turn_angle > 180) ? (turn_angle - 360) : turn_angle;

  left_motor_speed = calculateLeftSpeed(error, turn_direction);
  right_motor_speed = calculateRightSpeed(error, turn_direction);

  left_motor_speed = constrain(left_motor_speed, MIN_PPM, MAX_PPM);
  right_motor_speed = constrain(right_motor_speed, MIN_PPM, MAX_PPM);

  left_esc.writeMicroseconds(left_motor_speed);
  right_esc.writeMicroseconds(right_motor_speed);
}

// === UTILS ===
int16_t getHeading() {
  compass.read();
  int16_t azimuth = compass.getAzimuth();
  if (azimuth < 0)
    azimuth += 360;
  return azimuth;
}

int16_t calculateTurnAngle(int16_t current, int16_t desired) {
  return (desired - current + 360) % 360;
}

int16_t calculateLeftSpeed(int16_t error, int16_t turn_direction) {
  int16_t left_speed =
      c_data.cruise_speed + (c_data.kp * error * turn_direction);
  left_speed = constrain(left_speed, MIN_PPM, c_data.max_speed);
  return left_speed;
}

int16_t calculateRightSpeed(int16_t error, int16_t turn_direction) {
  int16_t right_speed =
      c_data.cruise_speed - (c_data.kp * error * turn_direction);
  right_speed = constrain(right_speed, MIN_PPM, c_data.max_speed);
  return right_speed;
}
