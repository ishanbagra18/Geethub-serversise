package helpers

import (
    "context"
    "log"
    "mime/multipart"
    "os"

    "github.com/cloudinary/cloudinary-go/v2"
    "github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// UploadFile uploads audio/image/video using io.Reader (correct)
func UploadFile(file multipart.File, fileHeader *multipart.FileHeader, folder string) (string, error) {

    // Reset file pointer before upload (VERY IMPORTANT)
    file.Seek(0, 0)

    // Initialize Cloudinary client
    cld, err := cloudinary.NewFromURL(os.Getenv("CLOUDINARY_URL"))
    if err != nil {
        log.Println("Cloudinary init error:", err)
        return "", err
    }

    // Correct upload using io.Reader (file stream)
    uploadResult, err := cld.Upload.Upload(context.Background(), file, uploader.UploadParams{
        Folder:       folder,
        ResourceType: "video", // needed for mp3/mp4 uploads
        PublicID:     fileHeader.Filename,
    })

    if err != nil {
        log.Println("Cloudinary upload error:", err)
        return "", err
    }

    return uploadResult.SecureURL, nil
}
