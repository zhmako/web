BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reputation INT NOT NULL DEFAULT 0,
    is_moderator BOOLEAN NOT NULL DEFAULT false,
    github_id TEXT UNIQUE,
    CONSTRAINT valid_username CHECK (username ~ '^[a-zA-Z0-9_-]{3,50}$')
);

CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    title VARCHAR(300) NOT NULL,
    content TEXT,
    url VARCHAR(2048),
    post_type VARCHAR(20) NOT NULL CHECK (post_type IN ('link', 'text', 'poll')),
    score INT NOT NULL DEFAULT 0,
    hotness FLOAT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_sponsored BOOLEAN NOT NULL DEFAULT false,
    tags VARCHAR(255)[]
);

CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    post_id UUID REFERENCES posts(id),
    parent_id UUID REFERENCES comments(id),
    content TEXT NOT NULL,
    score INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    path LTREE NOT NULL
);

CREATE TABLE votes (
    user_id UUID NOT NULL REFERENCES users(id),
    post_id UUID REFERENCES posts(id),
    comment_id UUID REFERENCES comments(id),
    value SMALLINT NOT NULL CHECK (value BETWEEN -1 AND 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, COALESCE(post_id, uuid_nil()), COALESCE(comment_id, uuid_nil()))
);

CREATE INDEX posts_hotness_idx ON posts USING BRIN (hotness);
CREATE INDEX posts_created_at_idx ON posts USING BRIN (created_at);
CREATE INDEX comments_path_idx ON comments USING GIST (path);
CREATE INDEX users_reputation_idx ON users (reputation);

COMMIT;