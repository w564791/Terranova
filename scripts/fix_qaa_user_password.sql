-- Fix qaa user password by hashing it with bcrypt
-- The plaintext password "aaaaaaaa" will be hashed using bcrypt

-- First, let's check the current state
SELECT user_id, username, password_hash, is_active 
FROM users 
WHERE username = 'qaa';

-- Update the password_hash with a properly bcrypt-hashed version of "aaaaaaaa"
-- This hash was generated using: bcrypt.GenerateFromPassword([]byte("aaaaaaaa"), bcrypt.DefaultCost)
-- The hash below is for password "aaaaaaaa" with bcrypt cost 10
UPDATE users 
SET password_hash = '$2a$10$YourBcryptHashHere'
WHERE username = 'qaa';

-- Note: You need to generate the actual bcrypt hash for "aaaaaaaa"
-- Run this Go code to generate it:
-- 
-- package main
-- import (
--     "fmt"
--     "golang.org/x/crypto/bcrypt"
-- )
-- func main() {
--     hash, _ := bcrypt.GenerateFromPassword([]byte("aaaaaaaa"), bcrypt.DefaultCost)
--     fmt.Println(string(hash))
-- }

-- Verify the update
SELECT user_id, username, password_hash, is_active 
FROM users 
WHERE username = 'qaa';
