package controllers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"               //Web framework for building APIs
	"github.com/go-playground/validator/v10" //Validates request body fields (email, required fields, etc.).

	"github.com/ishanbagra18/ecommerce-using-go/database" //Your MongoDB connection file.
	"github.com/ishanbagra18/ecommerce-using-go/helpers"  //Custom helper functions (e.g. JWT token generation).
	"github.com/ishanbagra18/ecommerce-using-go/models"   //Your data models (User struct, product struct, etc.).

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

var usercollection *mongo.Collection

func InitUserController() {
	usercollection = database.GetCollection("ecommerce", "users")
}

var validate = validator.New()

// HashPassword hashes a plain password
func HashPassword(password string) string {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		log.Panic(err)
	}
	return string(hashedPassword)
}

// VerifyPassword compares hashed password with plain text
func VerifyPassword(userPassword string, providedPassword string) (bool, string) {
	err := bcrypt.CompareHashAndPassword([]byte(userPassword), []byte(providedPassword))
	check := true
	msg := ""

	if err != nil {
		msg = "email or password is incorrect"
		check = false
	}
	return check, msg
}

// Signup controller (with debugging and fixes)
func Signup() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔍 [Signup] Endpoint hit")

		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		if err := c.BindJSON(&user); //Yeh user ke signup form ka JSON data lekar user struct me store kar deta hai.
		// Agar kisi field ka type galat ho → BindJSON error deta hai.

		err != nil {
			log.Println("❌ [Signup] BindJSON error:", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Printf("🔍 [Signup] Payload received: %+v\n", user)

		if validationErr := validate.Struct(user); validationErr != nil {
			log.Println("❌ [Signup] Validation error:", validationErr)
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
			return
		}
		log.Println("✅ [Signup] Validation passed")

		count, err := usercollection.CountDocuments(ctx, bson.M{"email": user.Email})
		if err != nil {
			log.Println("❌ [Signup] Error checking email:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("🔍 [Signup] Email count: %d\n", count)

		countPhone, err := usercollection.CountDocuments(ctx, bson.M{"phone": user.Phone})
		if err != nil {
			log.Println("❌ [Signup] Error checking phone:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("🔍 [Signup] Phone count: %d\n", countPhone)

		if count > 0 || countPhone > 0 {
			log.Println("❌ [Signup] Duplicate email or phone")
			c.JSON(http.StatusConflict, gin.H{"error": "email or phone already exists"})
			return
		}

		password := HashPassword(*user.Password)
		if password == "" {
			log.Println("❌ [Signup] Password hashing failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "password hashing failed"})
			return
		}
		user.Password = &password
		log.Println("✅ [Signup] Password hashed")

		now := time.Now()
		user.Created_at = &now
		user.Updated_at = &now
		user.ID = primitive.NewObjectID()
		user.User_id = user.ID.Hex()

		log.Println("🔍 [Signup] Generating tokens...")
		token, refreshToken, err := helpers.GenerateAllTokens(
			*user.Email, *user.First_name, *user.Last_name, *user.User_type, user.User_id,
		)
		if err != nil {
			log.Println("❌ [Signup] Token generation failed:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
			return
		}
		user.Token = &token
		user.Refresh_token = &refreshToken
		log.Println("✅ [Signup] Tokens generated")

		_, insertErr := usercollection.InsertOne(ctx, user)
		if insertErr != nil {
			log.Println("❌ [Signup] InsertOne error:", insertErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user not created"})
			return
		}

		log.Println("✅ [Signup] User inserted into MongoDB")
		c.JSON(http.StatusOK, gin.H{
			"msg":           "user created successfully",
			"token":         token,
			"refresh_token": refreshToken,
			"user":          user,
		})
	}
}

// Login controller
func Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		var foundUser models.User

		if err := c.BindJSON(&user); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := usercollection.FindOne(ctx, bson.M{"email": user.Email}).Decode(&foundUser)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "email or password is incorrect"})
			return
		}

		if foundUser.Password == nil || user.Password == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
			return
		}

		passwordIsValid, msg := VerifyPassword(*foundUser.Password, *user.Password)
		if !passwordIsValid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": msg})
			return
		}

		token, refreshToken, err := helpers.GenerateAllTokens(
			*foundUser.Email, *foundUser.First_name, *foundUser.Last_name, *foundUser.User_type, foundUser.User_id,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
			return
		}

		err = helpers.UpdateAllTokens(token, refreshToken, foundUser.User_id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tokens"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":         token,
			"refresh_token": refreshToken,
			"user":          foundUser,
		})
	}
}

func UpdateProfile() gin.HandlerFunc {
	return func(C *gin.Context) {

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		if err := C.BindJSON(&user); err != nil {
			C.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		userId := C.Param("user_id")
		if err := helpers.MatchUserTypeToUid(C, userId); err != nil {
			C.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		updateObj := bson.M{}

		if user.First_name != nil {
			updateObj["first_name"] = user.First_name
		}

		if user.Last_name != nil {
			updateObj["last_name"] = user.Last_name
		}

		if user.Email != nil {
			updateObj["email"] = user.Email
		}

		if user.Phone != nil {
			updateObj["phone"] = user.Phone
		}

		updateObj["updated_at"] = time.Now()

		filter := bson.M{"user_id": userId}
		update := bson.M{"$set": updateObj}

		var err error
		_, err = usercollection.UpdateOne(ctx, filter, update)
		if err != nil {
			C.JSON(http.StatusInternalServerError, gin.H{"error": "error while updating user profile"})
			return
		}

		var updatedUser models.User
		err = usercollection.FindOne(ctx, bson.M{"user_id": userId}).Decode(&updatedUser)
		if err != nil {
			C.JSON(http.StatusInternalServerError, gin.H{"error": "error while fetching updated user profile"})
			return
		}

		C.JSON(http.StatusOK, updatedUser)

	}
}

func MyProfile() gin.HandlerFunc {
	return func(ctx *gin.Context) {

		// Context with timeout for MongoDB operation
		c, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		// Get user_id from URL params
		userId := ctx.Param("user_id")
		if userId == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
			return
		}

		var user models.User

		// Find user in DB using `user_id` field
		err := usercollection.FindOne(c, bson.M{"user_id": userId}).Decode(&user)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			} else {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": "error fetching user"})
			}
			return
		}

		// Success response
		ctx.JSON(http.StatusOK, gin.H{
			"message": "User profile fetched successfully",
			"user":    user,
		})
	}
}

// Logout controller
func Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		// Get user_id from request (could come from token or URL param)
		userId := c.Param("user_id")
		if userId == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
			return
		}

		// Match user type & UID (authorization check)
		if err := helpers.MatchUserTypeToUid(c, userId); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		// Clear token and refresh_token in DB
		update := bson.M{
			"$set": bson.M{
				"token":         "",
				"refresh_token": "",
			},
		}

		_, err := usercollection.UpdateOne(ctx, bson.M{"user_id": userId}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error logging out"})
			return
		}

		// Optionally, if you are setting cookies, clear them here:
		// c.SetCookie("token", "", -1, "/", "", false, true)

		c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
	}
}

func ChangePassword() gin.HandlerFunc {
	return func(c *gin.Context) {

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		// 🔐 Logged-in user ID from token
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		var request struct {
			NewPassword string `json:"new_password" binding:"required,min=6"`
		}

		if err := c.BindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hashedPassword := HashPassword(request.NewPassword)

		update := bson.M{
			"$set": bson.M{
				"password": hashedPassword,
			},
		}

		_, err := usercollection.UpdateOne(
			ctx,
			bson.M{"user_id": userID.(string)},
			update,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Password changed successfully"})
	}
}

// GetUsers controller
func GetUsers() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := helpers.MatchUserTypeToUid(c, "ADMIN"); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		recordPerPage, err := strconv.Atoi(c.Query("recordPerPage"))
		if err != nil || recordPerPage < 1 {
			recordPerPage = 10
		}

		page, err := strconv.Atoi(c.Query("page"))
		if err != nil || page < 1 {
			page = 1
		}

		startIndex := (page - 1) * recordPerPage

		matchStage := bson.D{{Key: "$match", Value: bson.D{{}}}}
		groupStage := bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "null"},
			{Key: "total_count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "data", Value: bson.D{{Key: "$push", Value: "$$ROOT"}}},
		}}}
		projectStage := bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "total_count", Value: 1},
			{Key: "user_items", Value: bson.D{
				{Key: "$slice", Value: []interface{}{"$data", startIndex, recordPerPage}},
			}},
		}}}

		cursor, err := usercollection.Aggregate(ctx, mongo.Pipeline{matchStage, groupStage, projectStage})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error fetching users"})
			return
		}

		var allUsers []bson.M
		if err = cursor.All(ctx, &allUsers); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error decoding users"})
			return
		}

		if len(allUsers) > 0 {
			c.JSON(http.StatusOK, allUsers[0])
			return
		}
		c.JSON(http.StatusOK, gin.H{"users": []string{}})
	}
}

// GetUser controller
func GetUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := c.Param("user_id")

		if err := helpers.MatchUserTypeToUid(c, userId); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var user models.User
		err := usercollection.FindOne(ctx, bson.M{"user_id": userId}).Decode(&user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error fetching user"})
			return
		}

		c.JSON(http.StatusOK, user)
	}
}
