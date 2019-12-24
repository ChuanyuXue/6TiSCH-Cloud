package model

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db  *sql.DB
	err error
)

func init() {
	dbAddr := fmt.Sprintf("root:%s@tcp(127.0.0.1:3306)/6tisch", os.Getenv("DBPasswd"))
	db, _ = sql.Open("mysql", dbAddr)
	for {
		if err := db.Ping(); err != nil {
			fmt.Println(err, ", retry in 10s...")
			time.Sleep(10 * time.Second)
		} else {
			break
		}
	}

	// https://github.com/go-sql-driver/mysql/issues/674
	db.SetMaxIdleConns(0)
}

// Node info for topology.
type Node struct {
	FirstAppear int    `json:"first_appear"`
	LastSeen    int    `json:"last_seen"`
	Gateway     string `json:"gateway"`
	SensorID    int    `json:"sensor_id"`
	Address     string `json:"address"`
	Parent      int    `json:"parent"`
	Eui64       string `json:"eui64"`
	Position    struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"position"`
	Type  string `json:"type"`
	Power string `json:"power"`
}

func GetGateway(timeRange int64) ([]string, error) {
	var gName string
	gList := make([]string, 0)

	rows, err := db.Query("select distinct GATEWAY_NAME from TOPOLOGY_DATA where LAST_SEEN>=?;", timeRange)
	if err != nil {
		return gList, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&gName)
		gList = append(gList, gName)
	}
	// for multi-gateway test
	gList = append(gList, "UCONN_GWX")
	return gList, nil
}

func GetTopology(gatewayName string, timeRange int64) ([]Node, error) {
	var n Node
	var rows *sql.Rows
	nodeList := make([]Node, 0)

	if gatewayName == "any" {
		rows, err = db.Query("select * from TOPOLOGY_DATA where LAST_SEEN>=? and FIRST_APPEAR in (select MAX(FIRST_APPEAR) from TOPOLOGY_DATA group by SENSOR_ID);", timeRange)
	} else {
		rows, err = db.Query("select * from TOPOLOGY_DATA where GATEWAY_NAME=? and LAST_SEEN>=? and FIRST_APPEAR in (select MAX(FIRST_APPEAR) from TOPOLOGY_DATA group by SENSOR_ID);", gatewayName, timeRange)
	}
	if err != nil {
		return nodeList, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&n.FirstAppear, &n.LastSeen, &n.Gateway, &n.SensorID,
			&n.Address, &n.Parent, &n.Eui64, &n.Position.Lat, &n.Position.Lng, &n.Type, &n.Power)
		nodeList = append(nodeList, n)
	}
	return nodeList, nil
}

// NWStatData is all sensor's basic network stat data of one gateway
type NWStatData struct {
	SensorID          int     `json:"sensor_id"`
	Gateway           string  `json:"gateway"`
	AVGRTT            float64 `json:"avg_rtt"`
	AvgMACTxTotalDiff float32 `json:"avg_mac_tx_total_diff"`
	AvgMACTxNoACKDiff float32 `json:"avg_mac_tx_noack_diff"`
	AvgAPPPERSentDiff float32 `json:"avg_app_per_sent_diff"`
	AvgAPPPERLostDiff float32 `json:"avg_app_per_lost_diff"`
}

func GetNWStat(gatewayName string, timeRange int64) ([]NWStatData, error) {
	var n NWStatData
	// query NW_DATA_SET_PER_UCONN
	var rows1 *sql.Rows
	// query NW_DATA_SET_LATENCY
	var rows2 *sql.Rows
	nList := make([]NWStatData, 0)

	if gatewayName == "any" {
		rows1, err = db.Query(`select SENSOR_ID, GATEWAY_NAME, AVG(MAC_TX_TOTAL_DIFF),
		AVG(MAC_TX_NOACK_DIFF), AVG(APP_PER_SENT_DIFF),AVG(APP_PER_LOST_DIFF) from NW_DATA_SET_PER_UCONN 
		where TIMESTAMP>=? group by SENSOR_ID`, timeRange)
		if err != nil {
			return nList, err
		}
		rows2, err = db.Query(`select SENSOR_ID, GATEWAY_NAME, AVG(RTT) from
			NW_DATA_SET_LATENCY where TIMESTAMP>=? group by SENSOR_ID`, timeRange)
	} else {
		rows1, err = db.Query(`select SENSOR_ID, GATEWAY_NAME, AVG(MAC_TX_TOTAL_DIFF),
		AVG(MAC_TX_NOACK_DIFF),AVG(APP_PER_SENT_DIFF),AVG(APP_PER_LOST_DIFF) from NW_DATA_SET_PER_UCONN 
		where GATEWAY_NAME=? and TIMESTAMP>=? group by SENSOR_ID`, gatewayName, timeRange)
		if err != nil {
			return nList, err
		}
		rows2, err = db.Query(`select SENSOR_ID, GATEWAY_NAME, AVG(RTT) from
			NW_DATA_SET_LATENCY where GATEWAY_NAME=? and TIMESTAMP>=? group by SENSOR_ID`, gatewayName, timeRange)
	}
	if err != nil {
		return nList, err
	}
	defer rows1.Close()
	// defer rows2.Close()

	for rows1.Next() {
		rows1.Scan(&n.SensorID, &n.Gateway, &n.AvgMACTxTotalDiff, &n.AvgMACTxNoACKDiff,
			&n.AvgAPPPERSentDiff, &n.AvgAPPPERLostDiff)
		nList = append(nList, n)
	}

	// merge
	for rows2.Next() {
		rows2.Scan(&n.SensorID, &n.Gateway, &n.AVGRTT)
		for i, v := range nList {
			if v.SensorID == n.SensorID && v.Gateway == n.Gateway {
				nList[i].AVGRTT = n.AVGRTT
				break
			}
		}
	}

	return nList, nil
}

// SensorNWStatData is each sensor's network statistic: average RSSi value
type SensorNWStatData struct {
	Timestamp int `json:"timestamp"`
	AvgRSSI   int `json:"avg_rssi"`
}

// SensorNWStatAdvData is each sensor's network statistic detail
type SensorNWStatAdvData struct {
	Timestamp      int `json:"timestamp"`
	MacTxTotalDiff int `json:"mac_tx_total_diff"`
	MacTxNoAckDiff int `json:"mac_tx_noack_diff"`
	AppPERSentDiff int `json:"app_per_sent_diff"`
	AppPERLostDiff int `json:"app_per_lost_diff"`
}

func GetNWStatByID(gatewayName, sensorID string, timeRange int64) ([]SensorNWStatData, error) {
	var s SensorNWStatData
	var rows *sql.Rows
	sList := make([]SensorNWStatData, 0)

	rows, err = db.Query(`select TIMESTAMP, AVG_RSSI from NW_DATA_SET_PER_UCONN 
			where GATEWAY_NAME=? and SENSOR_ID=? and TIMESTAMP>=?`, gatewayName, sensorID, timeRange)
	if err != nil {
		return sList, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&s.Timestamp, &s.AvgRSSI)
		sList = append(sList, s)
	}

	return sList, nil
}

func GetNWStatAdvByID(gatewayName, sensorID string, timeRange int64) ([]SensorNWStatAdvData, error) {
	var s SensorNWStatAdvData
	var rows *sql.Rows
	sList := make([]SensorNWStatAdvData, 0)

	rows, err = db.Query(`select TIMESTAMP, MAC_TX_TOTAL_DIFF,
			MAC_TX_NOACK_DIFF,APP_PER_SENT_DIFF,APP_PER_LOST_DIFF from NW_DATA_SET_PER_UCONN 
			where GATEWAY_NAME=? and SENSOR_ID=? and TIMESTAMP>=?`, gatewayName, sensorID, timeRange)
	if err != nil {
		return sList, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&s.Timestamp, &s.MacTxTotalDiff, &s.MacTxNoAckDiff,
			&s.AppPERSentDiff, &s.AppPERLostDiff)
		sList = append(sList, s)
	}

	return sList, nil
}

type SensorBatteryData struct {
	Gateway         string  `json:"gateway"`
	SensorID        int     `json:"sensor_id"`
	AvgCC2650Active float64 `json:"avg_cc2650_active"`
	AvgCC2650Sleep  float64 `json:"avg_cc2650_sleep"`
	AvgRFRx         float64 `json:"avg_rf_rx"`
	AvgRFTx         float64 `json:"avg_rf_tx"`
	BatRemain       string  `json:"bat_remain"`
}

func GetBattery(gatewayName string, timeRange int64) ([]SensorBatteryData, error) {
	var b SensorBatteryData
	var rows *sql.Rows
	bList := make([]SensorBatteryData, 0)
	var bat float64
	if gatewayName == "any" {
		rows, err = db.Query(`select SENSOR_ID,GATEWAY_NAME,AVG(CC2650_ACTIVE),AVG(CC2650_SLEEP),AVG(RF_RX),AVG(RF_TX),BAT 
			from SENSOR_DATA where TIMESTAMP>=? group by SENSOR_ID`, timeRange)
	} else {
		rows, err = db.Query(`select SENSOR_ID,GATEWAY_NAME,AVG(CC2650_ACTIVE),AVG(CC2650_SLEEP),AVG(RF_RX),AVG(RF_TX),BAT 
			from SENSOR_DATA where GATEWAY_NAME=? and TIMESTAMP>=? group by SENSOR_ID`, gatewayName, timeRange)
	}
	if err != nil {
		return bList, err
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&b.SensorID, &b.Gateway, &b.AvgCC2650Active, &b.AvgCC2650Sleep, &b.AvgRFRx, &b.AvgRFTx, &bat)
		// todo
		// b.BatRemain = string(bat)
		bList = append(bList, b)
	}
	return bList, nil
}

type NoiseLevelData struct {
	Gateway    string  `json:"gateway"`
	SensorID   int     `json:"sensor_id"`
	NoiseLevel float64 `json:"noise_level"`
	Position   struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"position"`
}

type PERData struct {
	SensorID             int
	AvgRSSI              float64
	AvgRxRSSI            float64
	MacRxTotalDiff       float64
	MacTxTotalDiff       float64
	MacTxNoACKDiff       float64
	MacTxLengthTotalDiff float64
}

func GetNoiseLevel(gatewayName string, timeRange int64) ([]NoiseLevelData, error) {
	var per PERData
	var nl NoiseLevelData

	perList := make([]PERData, 0)
	nlList := make([]NoiseLevelData, 0)
	nodeList, err := GetTopology(gatewayName, timeRange)
	if err != nil {
		return nlList, err
	}

	rows, err := db.Query("SELECT SENSOR_ID,AVG(AVG_RSSI),AVG(AVG_RXRSSI),AVG(MAC_RX_TOTAL_DIFF),AVG(MAC_TX_NOACK_DIFF),AVG(MAC_TX_TOTAL_DIFF),AVG(MAC_TX_LENGTH_TOTAL_DIFF) FROM NW_DATA_SET_PER_UCONN GROUP BY SENSOR_ID")
	if err != nil {
		return nlList, err
	}
	for rows.Next() {
		rows.Scan(&per.SensorID, &per.AvgRSSI, &per.AvgRxRSSI, &per.MacRxTotalDiff, &per.MacTxNoACKDiff, &per.MacTxTotalDiff, &per.MacTxLengthTotalDiff)
		perList = append(perList, per)
	}

	for _, p := range perList {
		acRssi := float64(p.AvgRSSI)
		acTx := float64(p.MacRxTotalDiff)
		acLost := float64(p.MacTxNoACKDiff - p.MacTxTotalDiff - p.MacRxTotalDiff)

		var (
			txRssi   float64
			txTx     float64
			txLost   float64
			txLength float64
		)

		var parent = 0
		for _, node := range nodeList {
			if p.SensorID == node.SensorID {
				nl.Position = node.Position
				parent = node.Parent
				break
			}
		}
		for _, pp := range perList {
			if pp.SensorID == parent {
				txRssi = pp.AvgRxRSSI
				txTx = pp.MacTxTotalDiff
				txLost = pp.MacTxTotalDiff - pp.MacRxTotalDiff
				txLength = pp.MacTxLengthTotalDiff
			}
		}
		var noiseLevel float64
		if acTx > 0 && txTx > 0 {
			acNosie := math.Pow(10, (noiseCompute(acTx, acLost, acRssi, 20) / 10))
			txNoise := math.Pow(10, (noiseCompute(txTx, txLost, txRssi, txLength/txTx) / 10))
			noiseLevel = 10 * math.Log10(txNoise*txTx/(txTx+acTx)+acNosie*acTx/(txTx+acTx))
		} else if acTx > 0 {
			noiseLevel = noiseCompute(acTx, acLost, acRssi, 20)
		} else if txTx > 0 {
			noiseLevel = math.Pow(10, (noiseCompute(txTx, txLost, txRssi, txLength/txTx) / 10))
		} else {
			noiseLevel = -99.0
		}
		nl.Gateway = gatewayName
		nl.SensorID = p.SensorID
		nl.NoiseLevel = noiseLevel
		nlList = append(nlList, nl)
	}

	return nlList, nil
}

// noise compute utils

func noiseCompute(txTotal, lostTotal, rssi, length float64) float64 {
	return rssi - snrDb(lostTotal/txTotal, length)
}

func snrDb(plrIn, length float64) float64 {
	midSNR := 0.000000
	maxSNR := 4.000000
	minSNR := -4.000000
	midPLR := plr(midSNR, length)

	for math.Abs(minSNR-maxSNR) > 0.00001 {
		midSNR = (maxSNR + minSNR) / 2
		midPLR = plr(midPLR, length)
		if math.Abs(plrIn-midPLR) < 0.00001 {
			return midSNR
		} else if plrIn > midPLR {
			maxSNR = midSNR - 0.000001
		} else if plrIn < midPLR {
			minSNR = midSNR + 0.000001
		}
	}
	return midSNR
}

func plr(snrIn, length float64) float64 {
	bitErrorRate := 0.5 * math.Erfc(math.Sqrt(math.Pow(10, (snrIn/10)))) * 1.45
	para := 0.0

	for i := 1; i < 33; i++ {
		para = para + fact(32)/(fact(i)*fact(32-i))*math.Pow(bitErrorRate, float64(i))*(math.Pow(1-bitErrorRate, float64(32-i)))*p(i)
	}
	return 1 - math.Pow((1-para), 2*length)
}

func fact(n int) float64 {
	res := 1
	for i := 2; i < n+1; i++ {
		res *= i
	}
	return float64(res)
}

func p(n int) float64 {
	if n <= 5 {
		return 0.000000
	} else if n == 6 {
		return 0.002000
	} else if n == 7 {
		return 0.013400
	} else if n == 8 {
		return 0.052300
	} else if n == 9 {
		return 0.149800
	} else if n == 10 {
		return 0.347900
	} else if n == 11 {
		return 0.649600
	} else if n == 12 {
		return 0.915600
	} else if n == 13 {
		return 0.996800
	} else {
		return 1.000000
	}
}
