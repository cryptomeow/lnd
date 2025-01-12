package macaroons

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"

	"github.com/cryptomeow/lnd/channeldb/kvdb"

	"github.com/btcsuite/btcwallet/snacl"
)

const (
	// RootKeyLen is the length of a root key.
	RootKeyLen = 32
)

var (
	// rootKeyBucketName is the name of the root key store bucket.
	rootKeyBucketName = []byte("macrootkeys")

	// DefaultRootKeyID is the ID of the default root key. The first is
	// just 0, to emulate the memory storage that comes with bakery.
	DefaultRootKeyID = []byte("0")

	// encryptedKeyID is the name of the database key that stores the
	// encryption key, encrypted with a salted + hashed password. The
	// format is 32 bytes of salt, and the rest is encrypted key.
	encryptedKeyID = []byte("enckey")

	// ErrAlreadyUnlocked specifies that the store has already been
	// unlocked.
	ErrAlreadyUnlocked = fmt.Errorf("macaroon store already unlocked")

	// ErrStoreLocked specifies that the store needs to be unlocked with
	// a password.
	ErrStoreLocked = fmt.Errorf("macaroon store is locked")

	// ErrPasswordRequired specifies that a nil password has been passed.
	ErrPasswordRequired = fmt.Errorf("a non-nil password is required")

	// ErrKeyValueForbidden is used when the root key ID uses encryptedKeyID as
	// its value.
	ErrKeyValueForbidden = fmt.Errorf("root key ID value is not allowed")
)

// RootKeyStorage implements the bakery.RootKeyStorage interface.
type RootKeyStorage struct {
	kvdb.Backend

	encKeyMtx sync.RWMutex
	encKey    *snacl.SecretKey
}

// NewRootKeyStorage creates a RootKeyStorage instance.
// TODO(aakselrod): Add support for encryption of data with passphrase.
func NewRootKeyStorage(db kvdb.Backend) (*RootKeyStorage, error) {
	// If the store's bucket doesn't exist, create it.
	err := kvdb.Update(db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(rootKeyBucketName)
		return err
	}, func() {})
	if err != nil {
		return nil, err
	}

	// Return the DB wrapped in a RootKeyStorage object.
	return &RootKeyStorage{Backend: db, encKey: nil}, nil
}

// CreateUnlock sets an encryption key if one is not already set, otherwise it
// checks if the password is correct for the stored encryption key.
func (r *RootKeyStorage) CreateUnlock(password *[]byte) error {
	r.encKeyMtx.Lock()
	defer r.encKeyMtx.Unlock()

	// Check if we've already unlocked the store; return an error if so.
	if r.encKey != nil {
		return ErrAlreadyUnlocked
	}

	// Check if a nil password has been passed; return an error if so.
	if password == nil {
		return ErrPasswordRequired
	}

	return kvdb.Update(r, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(rootKeyBucketName)
		dbKey := bucket.Get(encryptedKeyID)
		if len(dbKey) > 0 {
			// We've already stored a key, so try to unlock with
			// the password.
			encKey := &snacl.SecretKey{}
			err := encKey.Unmarshal(dbKey)
			if err != nil {
				return err
			}

			err = encKey.DeriveKey(password)
			if err != nil {
				return err
			}

			r.encKey = encKey
			return nil
		}

		// We haven't yet stored a key, so create a new one.
		encKey, err := snacl.NewSecretKey(
			password, scryptN, scryptR, scryptP,
		)
		if err != nil {
			return err
		}

		err = bucket.Put(encryptedKeyID, encKey.Marshal())
		if err != nil {
			return err
		}

		r.encKey = encKey
		return nil
	}, func() {})
}

// Get implements the Get method for the bakery.RootKeyStorage interface.
func (r *RootKeyStorage) Get(_ context.Context, id []byte) ([]byte, error) {
	r.encKeyMtx.RLock()
	defer r.encKeyMtx.RUnlock()

	if r.encKey == nil {
		return nil, ErrStoreLocked
	}
	var rootKey []byte
	err := kvdb.View(r, func(tx kvdb.RTx) error {
		dbKey := tx.ReadBucket(rootKeyBucketName).Get(id)
		if len(dbKey) == 0 {
			return fmt.Errorf("root key with id %s doesn't exist",
				string(id))
		}

		decKey, err := r.encKey.Decrypt(dbKey)
		if err != nil {
			return err
		}

		rootKey = make([]byte, len(decKey))
		copy(rootKey[:], decKey)
		return nil
	}, func() {
		rootKey = nil
	})
	if err != nil {
		return nil, err
	}

	return rootKey, nil
}

// RootKey implements the RootKey method for the bakery.RootKeyStorage
// interface.
func (r *RootKeyStorage) RootKey(ctx context.Context) ([]byte, []byte, error) {
	r.encKeyMtx.RLock()
	defer r.encKeyMtx.RUnlock()

	if r.encKey == nil {
		return nil, nil, ErrStoreLocked
	}
	var rootKey []byte

	// Read the root key ID from the context. If no key is specified in the
	// context, an error will be returned.
	id, err := RootKeyIDFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	if bytes.Equal(id, encryptedKeyID) {
		return nil, nil, ErrKeyValueForbidden
	}

	err = kvdb.Update(r, func(tx kvdb.RwTx) error {
		ns := tx.ReadWriteBucket(rootKeyBucketName)
		dbKey := ns.Get(id)

		// If there's a root key stored in the bucket, decrypt it and
		// return it.
		if len(dbKey) != 0 {
			decKey, err := r.encKey.Decrypt(dbKey)
			if err != nil {
				return err
			}

			rootKey = make([]byte, len(decKey))
			copy(rootKey[:], decKey[:])
			return nil
		}

		// Otherwise, create a RootKeyLen-byte root key, encrypt it,
		// and store it in the bucket.
		rootKey = make([]byte, RootKeyLen)
		if _, err := io.ReadFull(rand.Reader, rootKey[:]); err != nil {
			return err
		}

		encKey, err := r.encKey.Encrypt(rootKey)
		if err != nil {
			return err
		}
		return ns.Put(id, encKey)
	}, func() {
		rootKey = nil
	})
	if err != nil {
		return nil, nil, err
	}

	return rootKey, id, nil
}

// Close closes the underlying database and zeroes the encryption key stored
// in memory.
func (r *RootKeyStorage) Close() error {
	r.encKeyMtx.Lock()
	defer r.encKeyMtx.Unlock()

	if r.encKey != nil {
		r.encKey.Zero()
	}
	return r.Backend.Close()
}

// ListMacaroonIDs returns all the root key ID values except the value of
// encryptedKeyID.
func (r *RootKeyStorage) ListMacaroonIDs(_ context.Context) ([][]byte, error) {
	r.encKeyMtx.RLock()
	defer r.encKeyMtx.RUnlock()

	// Check it's unlocked.
	if r.encKey == nil {
		return nil, ErrStoreLocked
	}

	var rootKeySlice [][]byte

	// Read all the items in the bucket and append the keys, which are the
	// root key IDs we want.
	err := kvdb.View(r, func(tx kvdb.RTx) error {

		// appendRootKey is a function closure that appends root key ID
		// to rootKeySlice.
		appendRootKey := func(k, _ []byte) error {
			// Only append when the key value is not encryptedKeyID.
			if !bytes.Equal(k, encryptedKeyID) {
				rootKeySlice = append(rootKeySlice, k)
			}
			return nil
		}

		return tx.ReadBucket(rootKeyBucketName).ForEach(appendRootKey)
	}, func() {
		rootKeySlice = nil
	})
	if err != nil {
		return nil, err
	}

	return rootKeySlice, nil
}

// DeleteMacaroonID removes one specific root key ID. If the root key ID is
// found and deleted, it will be returned.
func (r *RootKeyStorage) DeleteMacaroonID(
	_ context.Context, rootKeyID []byte) ([]byte, error) {

	r.encKeyMtx.RLock()
	defer r.encKeyMtx.RUnlock()

	// Check it's unlocked.
	if r.encKey == nil {
		return nil, ErrStoreLocked
	}

	// Check the rootKeyID is not empty.
	if len(rootKeyID) == 0 {
		return nil, ErrMissingRootKeyID
	}

	// Deleting encryptedKeyID or DefaultRootKeyID is not allowed.
	if bytes.Equal(rootKeyID, encryptedKeyID) ||
		bytes.Equal(rootKeyID, DefaultRootKeyID) {

		return nil, ErrDeletionForbidden
	}

	var rootKeyIDDeleted []byte
	err := kvdb.Update(r, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(rootKeyBucketName)

		// Check the key can be found. If not, return nil.
		if bucket.Get(rootKeyID) == nil {
			return nil
		}

		// Once the key is found, we do the deletion.
		if err := bucket.Delete(rootKeyID); err != nil {
			return err
		}
		rootKeyIDDeleted = rootKeyID

		return nil
	}, func() {
		rootKeyIDDeleted = nil
	})
	if err != nil {
		return nil, err
	}

	return rootKeyIDDeleted, nil
}
