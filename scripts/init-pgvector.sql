-- PostgreSQL with pgvector initialization script
-- This script initializes the pgvector extension and creates the database schema

-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Enable other useful extensions
CREATE EXTENSION IF NOT EXISTS pg_trgm;  -- For text similarity search
CREATE EXTENSION IF NOT EXISTS btree_gin; -- For GIN indexes on scalar types

-- Log successful initialization
DO $$
BEGIN
    RAISE NOTICE 'pgvector extension initialized successfully';
    RAISE NOTICE 'PostgreSQL version: %', version();
END $$;
