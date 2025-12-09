package util

import "math"

func DistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Bán kính trái đất (mét)
	dLat := Deg2rad(lat2 - lat1)
	dLon := Deg2rad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(Deg2rad(lat1))*math.Cos(Deg2rad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	rad1 := Deg2rad(lat1)
	rad2 := Deg2rad(lat2)
	delta := Deg2rad(lon2 - lon1)

	y := math.Sin(delta) * math.Cos(rad2)
	x := math.Cos(rad1)*math.Sin(rad2) -
		math.Sin(rad1)*math.Cos(rad2)*math.Cos(delta)

	ans := math.Atan2(y, x)
	return NormalizeAngle(Rad2deg(ans))
}

func Deg2rad(d float64) float64 { return d * math.Pi / 180 }
func Rad2deg(r float64) float64 { return r * 180 / math.Pi }

func NormalizeAngle(a float64) float64 {
	a = math.Mod(a+360, 360)
	if a < 0 {
		a += 360
	}
	return a
}

func Clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
