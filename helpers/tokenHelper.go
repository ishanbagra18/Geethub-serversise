package helpers

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/ishanbagra18/ecommerce-using-go/database"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type SignedDetails struct {
	Email      string
	First_name string
	Last_name  string
	Uid        string
	User_type  string
	jwt.StandardClaims
}

var usercollection *mongo.Collection

func InitUserController() {
	usercollection = database.GetCollection("ecommerce", "users")
}

var SECRET_KEY string = os.Getenv("SECRET_KEY")

// GenerateAllTokens creates access and refresh tokens for a user
func GenerateAllTokens(email string, firstName string, lastName string, userType string, uid string) (signedToken string, signedRefreshToken string, err error) {
	claims := &SignedDetails{
		Email:      email,
		First_name: firstName,
		Last_name:  lastName,
		Uid:        uid,
		User_type:  userType,
		StandardClaims: jwt.StandardClaims{
ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),  // 24 hours
		},
	}

	refreshClaims := &SignedDetails{
		StandardClaims: jwt.StandardClaims{
ExpiresAt: time.Now().Add(time.Hour * 24 * 30).Unix(),  // 30 days refresh token
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(SECRET_KEY))
	if err != nil {
		log.Println("❌ GenerateAllTokens: token signing error:", err)
		return "", "", err
	}

	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(SECRET_KEY))
	if err != nil {
		log.Println("❌ GenerateAllTokens: refresh token signing error:", err)
		return "", "", err
	}

	return token, refreshToken, nil
}

// ValidateToken verifies a token and returns its claims
func ValidateToken(signedToken string) (claims *SignedDetails, err error) {
	token, err := jwt.ParseWithClaims(
		signedToken,
		&SignedDetails{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte(SECRET_KEY), nil
		},
	)

	if err != nil {
		err = fmt.Errorf("the token is invalid: %v", err)
		return
	}

	claims, ok := token.Claims.(*SignedDetails)
	if !ok {
		err = fmt.Errorf("the token is invalid")
		return
	}

	if claims.ExpiresAt < time.Now().Unix() {
		err = fmt.Errorf("token is expired")
		return
	}

	return claims, nil
}

// UpdateAllTokens updates access and refresh tokens in the database
func UpdateAllTokens(signedToken string, signedRefreshToken string, userId string) error {
	var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	updateObj := bson.D{
		{"$set", bson.D{
			{"token", signedToken},
			{"refresh_token", signedRefreshToken},
			{"updated_at", time.Now()},
		}},
	}

	filter := bson.M{"user_id": userId}

	_, err := usercollection.UpdateOne(ctx, filter, updateObj)
	if err != nil {
		log.Println("❌ UpdateAllTokens: failed to update tokens:", err)
	}
	return err
}
