package main

import (
	"encoding/base64"
	"log"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func main() {
	// Generate a new ECDSA private key
	priv, _, err := crypto.GenerateKeyPair(crypto.ECDSA, 2048)
	if err != nil {
		log.Fatal(err)
	}

	// Marshal the private key to bytes
	privBytes, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		log.Fatal(err)
	}

	// Encode in base64
	privB64 := base64.StdEncoding.EncodeToString(privBytes)
	log.Printf("Generated private key: %s", privB64)

	// Get the peer ID
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Peer ID: %s", id)
}