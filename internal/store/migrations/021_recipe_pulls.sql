CREATE TABLE recipe_pulls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recipe_id TEXT NOT NULL,
    pulled_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_recipe_pulls_recipe_id ON recipe_pulls(recipe_id);
CREATE INDEX idx_recipe_pulls_pulled_at ON recipe_pulls(pulled_at);
