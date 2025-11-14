package main

import (
  "image"
  
  "github.com/skip2/go-qrcode"
)

// generateQRCodeImage generates a QR code image from a string using the go-qrcode library
func generateQRCodeImage(data string) (image.Image, error) {
  // Use the go-qrcode library to generate a proper QR code
  qr, err := qrcode.New(data, qrcode.Medium)
  if err != nil {
    return nil, err
  }
  
  // Generate image at 512x512 pixels
  qr.DisableBorder = false
  img := qr.Image(512)
  
  return img, nil
}

// generateASCIIQR generates an ASCII art representation of a QR code
func generateASCIIQR(data string) string {
  // Use the go-qrcode library to generate a proper ASCII QR code
  qr, err := qrcode.New(data, qrcode.Medium)
  if err != nil {
    return "Error generating QR code"
  }
  
  // Convert to ASCII art (using Unicode blocks for better visibility)
  return qr.ToSmallString(false)
}

