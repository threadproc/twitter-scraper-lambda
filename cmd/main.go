package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	twv1 "github.com/dghubble/go-twitter/twitter"
	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	twitterscraper "github.com/threadproc/twitter-scraper-lambda"
)

var ginLambda *ginadapter.GinLambda
var ts *twitterscraper.Scraper

func errResponse(c *gin.Context, code int, err error) {
	hub := sentrygin.GetHubFromContext(c)
	if code >= 500 {
		// we only want to capture 5xx errors here
		hub.CaptureException(err)
	}

	log.WithError(err).Error("error in response")
	c.JSON(code, map[string]interface{}{
		"error": err.Error(),
	})
}

func handleTweets(c *gin.Context) {
	idParam := c.Request.URL.Query().Get("tweet_ids")
	if len(idParam) == 0 {
		errResponse(c, 400, errors.New("you must provide tweet_ids param"))
		return
	}
	tweetIds := strings.Split(idParam, ",")
	ids := make([]uint64, len(tweetIds))

	var err error
	for i, id := range tweetIds {
		ids[i], err = strconv.ParseUint(id, 10, 64)
		if err != nil {
			errResponse(c, 400, err)
			return
		}
	}

	if len(tweetIds) > 50 {
		errResponse(c, 400, errors.New("no more than 50 tweets can be processed at a time"))
		return
	}

	tweets := make(map[string]*twitterscraper.ScrapedTweet)
	users := make(map[string]*twv1.User)

	for _, id := range ids {
		tws, usrs, err := ts.GetAllFromTweet(id)
		if err != nil {
			errResponse(c, 500, err)
			return
		}

		for _, tw := range tws {
			tweets[tw.IDStr] = tw
		}
		for _, usr := range usrs {
			users[usr.IDStr] = usr
		}
	}

	c.JSON(200, twitterscraper.ScrapeResponse{
		Tweets: tweets,
		Users:  users,
	})
}

func init() {
	log.Info("🚀 Cold-starting twitter-scraper-lambda")

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              os.Getenv("SENTRY_DSN"),
		TracesSampleRate: 1.0,
	}); err != nil {
		panic(err.Error())
	}

	ts = twitterscraper.NewScraper()

	r := gin.Default()

	r.Use(sentrygin.New(sentrygin.Options{
		Repanic: true,
	}))

	r.GET("/tweet", handleTweets)
	r.GET("/user", func(c *gin.Context) {
		c.String(200, "get user")
	})

	ginLambda = ginadapter.New(r)
}

func Handler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	return ginLambda.ProxyWithContext(ctx, req)
}

func main() {
	log.Info("🚀 Starting twitter-scraper-lambda")
	lambda.Start(Handler)
}
