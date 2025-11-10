FROM sgroups/goose:v3.23.1

# Copy migrations from root migrations directory (complete schema)
COPY migrations/ /app/
