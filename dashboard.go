package main

import (
	"btcminerproxy/config"
	"btcminerproxy/dash"
	"btcminerproxy/venuslog"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type chartData struct {
	Labels []string  `json:"labels"`
	Data   []float64 `json:"data"`
	Miners []int     `json:"miners"`
}

func timeSince(epoch int64) string {
	deltaT := time.Now().Unix() - epoch

	if deltaT > 3600 {
		return fmt.Sprintf("%dh", deltaT/3600)
	} else if deltaT > 60 {
		return fmt.Sprintf("%dm", deltaT/60)
	} else {
		return fmt.Sprintf("%ds", deltaT)
	}
}

func StartDashboard() {

	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.Data(200, "text/html", dash.MainPage)
	})

	r.GET("/stats", func(c *gin.Context) {
		getStats()
		c.JSON(200, gin.H{
			"hr":        avgHashrate,
			"miners":    numMiners,
			"upstreams": numUpstreams,
		})
	})

	r.GET("/hr_chart", func(c *gin.Context) {
		c.JSON(200, hrChart)
	})

	r.GET("/hr_chart_js", func(c *gin.Context) {
		cd := chartData{
			Labels: make([]string, 0, 288),
			Data:   make([]float64, 0, 288),
			Miners: make([]int, 0, 288),
		}

		for _, v := range hrChart {
			cd.Labels = append(cd.Labels, timeSince(v.Time))
			cd.Data = append(cd.Data, math.Round(v.Hr/10)/100)
			cd.Miners = append(cd.Miners, v.Miners)
		}

		c.JSON(200, cd)
	})

	r.GET("/configuration", func(c *gin.Context) {
		c.JSON(200, config.CFG)
	})

	r.GET("/disconnect", func(c *gin.Context) {
		remoteAddr := c.Query("miner")

		disconnectMiner(remoteAddr)

		c.JSON(200, gin.H{
			"result": "ok",
		})
	})

	r.GET("/addWhite", func(c *gin.Context) {
		remoteAddr := c.Query("addr")

		addList(remoteAddr, true)

		c.JSON(200, gin.H{
			"result": "ok",
		})
	})

	r.GET("/delWhite", func(c *gin.Context) {
		remoteAddr := c.Query("addr")

		delList(remoteAddr, true)

		c.JSON(200, gin.H{
			"result": "ok",
		})
	})

	r.GET("/addBlack", func(c *gin.Context) {
		remoteAddr := c.Query("addr")

		addList(remoteAddr, false)

		disconnectMiner(remoteAddr)

		c.JSON(200, gin.H{
			"result": "ok",
		})
	})

	r.GET("/delBlack", func(c *gin.Context) {
		remoteAddr := c.Query("addr")

		delList(remoteAddr, false)

		c.JSON(200, gin.H{
			"result": "ok",
		})
	})

	r.GET("/getWhiteList", func(c *gin.Context) {

		c.JSON(200, gin.H{
			"list": getList(true),
		})
	})

	r.GET("/getBlackList", func(c *gin.Context) {

		c.JSON(200, gin.H{
			"list": getList(false),
		})
	})

	r.GET("/getPoolList", func(c *gin.Context) {

		c.JSON(200, gin.H{
			"list":       getPoolList(),
			"currentIdx": config.CFG.PoolIndex,
		})
	})

	r.GET("/setPool", func(c *gin.Context) {

		poolIndexStr := c.Query("pool")
		poolIndex, err := strconv.Atoi(poolIndexStr)

		if err != nil {
			c.JSON(200, gin.H{
				"error": err,
			})
			return
		}

		c.JSON(200, gin.H{
			"list": setPool(uint64(poolIndex)),
		})
	})

	r.GET("/addPool", func(c *gin.Context) {

		// {
		// 	"url": "stratum.antpool.com:3333",
		// 	"tls": false,
		// 	"user": "montyanderson.pc",
		// 	"pass": "x"
		// },

		poolUrl := c.Query("url")
		poolTls, _ := strconv.ParseBool(c.Query("tls"))
		poolUser := c.Query("user")
		poolPass := c.Query("pass")

		venuslog.Warn("poolUrl", poolUrl)
		venuslog.Warn("poolTls", poolTls)
		venuslog.Warn("poolUser", poolUser)
		venuslog.Warn("poolPass", poolPass)

		newPool := config.PoolInfo{
			Url:            poolUrl,
			Tls:            poolTls,
			TlsFingerprint: "",
			User:           poolUser,
			Pass:           poolPass,
		}

		config.CFG.Pools = append(config.CFG.Pools, newPool)

		str, _ := json.Marshal(config.CFG.Pools)
		venuslog.Warn("newPool", string(str))

		c.JSON(200, gin.H{
			"list": getPoolList(),
		})
	})

	r.GET("/report", func(c *gin.Context) {

		c.JSON(200, gin.H{
			"list": getReport(),
		})
	})

	r.Run(fmt.Sprintf("%s:%d", config.CFG.Dashboard.Host, config.CFG.Dashboard.Port))
}
