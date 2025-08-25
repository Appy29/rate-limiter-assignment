#!/bin/bash

# Token for stress test
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoic3RyZXNzX3Rlc3RfdXNlciIsInN1YiI6InN0cmVzc190ZXN0X3VzZXIiLCJleHAiOjE3NTU4Njc1MjUsImlhdCI6MTc1NTc4MTEyNX0.-Y21hBrjFz24x8NjUuK08jy1pTiYgFcl-mGTRIUAomU"

echo "Starting 150 simultaneous requests from terminal: $$"

# Fire 150 requests simultaneously
for i in {1..150}; do
  {
    response=$(curl -s -X POST http://localhost:8080/acquire \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"tokens": 1}')
    
    allowed=$(echo $response | grep -o '"allowed":[^,]*' | cut -d':' -f2 | tr -d ',' | tr -d ' ')
    
    if [ "$allowed" = "false" ]; then
      echo "Terminal $$ - Request $i: íº¨ RATE LIMITED"
    else
      echo "Terminal $$ - Request $i: âœ… allowed"
    fi
  } &
done

# Wait for all background jobs to complete
wait

echo "Terminal $$ completed all 150 requests"
