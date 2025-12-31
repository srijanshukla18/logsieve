-- LogSieve Load Test Script for wrk
-- Usage: wrk -t4 -c100 -d30s -s test/load/basic.lua http://localhost:8080

-- Configuration
local counter = 0
local log_templates = {
    'User %d logged in from 192.168.1.%d',
    'Request processed in %dms for /api/v1/users/%d',
    'Connection established to database server 10.0.0.%d:%d',
    'Health check passed: %dms latency',
    'Cache hit ratio: %d%% for key user_%d',
    'ERROR: Connection refused to service on port %d',
    'WARN: High memory usage detected: %d%%',
    'DEBUG: Processing request #%d from client %d',
    'INFO: Batch job completed: %d records processed in %dms',
    'FATAL: Out of memory error in process %d'
}

local levels = {'DEBUG', 'INFO', 'WARN', 'ERROR', 'FATAL'}

-- Generate random IP
function random_ip()
    return string.format("%d.%d.%d.%d",
        math.random(1, 255),
        math.random(0, 255),
        math.random(0, 255),
        math.random(1, 254))
end

-- Generate a single log entry
function generate_log()
    counter = counter + 1
    local template_idx = (counter % #log_templates) + 1
    local template = log_templates[template_idx]

    -- Generate log message with random values
    local message = string.format(template,
        math.random(1, 10000),
        math.random(1, 255),
        math.random(100, 9999),
        math.random(1, 1000))

    local level = levels[(counter % #levels) + 1]
    local timestamp = os.date("!%Y-%m-%dT%H:%M:%S") .. "Z"

    return string.format([[{
        "log": "%s %s [main] com.example.App - %s",
        "time": "%s",
        "labels": {
            "container_name": "app-%d",
            "image": "java:17",
            "level": "%s"
        }
    }]], timestamp, level, message, timestamp, (counter % 10) + 1, level)
end

-- Generate batch of logs
function generate_batch(size)
    local logs = {}
    for i = 1, size do
        table.insert(logs, string.format([[{"message": "%s", "timestamp": "%s"}]],
            string.format(log_templates[(i % #log_templates) + 1],
                math.random(1, 10000),
                math.random(1, 255)),
            os.date("!%Y-%m-%dT%H:%M:%S") .. "Z"))
    end
    return '{"logs": [' .. table.concat(logs, ',') .. ']}'
end

-- Request setup
function setup(thread)
    thread:set("id", thread.id)
end

-- Request generation
function request()
    counter = counter + 1

    -- Mix of single logs and batches
    local body
    local path = "/ingest"

    if counter % 5 == 0 then
        -- Every 5th request is a batch
        body = generate_batch(10)
    else
        -- Single log entry
        body = generate_log()
    end

    local headers = {
        ["Content-Type"] = "application/json",
        ["X-Source"] = "load-test"
    }

    return wrk.format("POST", path, headers, body)
end

-- Response handling
function response(status, headers, body)
    if status ~= 200 then
        print("Error: " .. status .. " - " .. body)
    end
end

-- Report generation
function done(summary, latency, requests)
    io.write("\n--- LogSieve Load Test Results ---\n")
    io.write(string.format("Requests:      %d\n", summary.requests))
    io.write(string.format("Errors:        %d\n", summary.errors.status + summary.errors.connect + summary.errors.read + summary.errors.write + summary.errors.timeout))
    io.write(string.format("Requests/sec:  %.2f\n", summary.requests / (summary.duration / 1000000)))
    io.write(string.format("Transfer/sec:  %.2f KB\n", (summary.bytes / 1024) / (summary.duration / 1000000)))
    io.write("\nLatency Distribution:\n")
    io.write(string.format("  50%%:  %.2fms\n", latency:percentile(50) / 1000))
    io.write(string.format("  75%%:  %.2fms\n", latency:percentile(75) / 1000))
    io.write(string.format("  90%%:  %.2fms\n", latency:percentile(90) / 1000))
    io.write(string.format("  99%%:  %.2fms\n", latency:percentile(99) / 1000))
    io.write("----------------------------------\n")
end
