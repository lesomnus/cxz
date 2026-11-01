WITH points AS (
  SELECT ResourceAttributes AS resource, Attributes AS attributes, MetricName AS metric,
         StartTimeUnix AS started, TimeUnix AS time, AggregationTemporality AS temporality,
         any(Value) AS value
  FROM telemetry.otel_metrics_sum
  WHERE TimeUnix < fromUnixTimestamp64Milli({to:Int64}) AND (%s)
    AND (startsWith(MetricName,'cxz.') OR MetricName IN ('claude_code.token.usage','claude_code.cost.usage'))
    AND ResourceAttributes['cxz.session.id'] NOT IN (SELECT session FROM telemetry.erased_sessions)
  GROUP BY resource,attributes,metric,started,time,temporality
  UNION ALL
  SELECT ResourceAttributes AS resource, Attributes AS attributes, MetricName AS metric,
         StartTimeUnix AS started, TimeUnix AS time, AggregationTemporality AS temporality,
         any(Sum) AS value
  FROM telemetry.otel_metrics_histogram
  WHERE TimeUnix < fromUnixTimestamp64Milli({to:Int64}) AND (%s)
    AND MetricName = 'codex.turn.token_usage'
    AND ResourceAttributes['cxz.session.id'] NOT IN (SELECT session FROM telemetry.erased_sessions)
  GROUP BY resource,attributes,metric,started,time,temporality
), changes AS (
  SELECT *, if(temporality=2,
    if(row_number() OVER stream > 1 AND value >= lagInFrame(value) OVER stream,
       value-lagInFrame(value) OVER stream,value),value) AS delta
  FROM points
  WINDOW stream AS (PARTITION BY resource,attributes,metric,started ORDER BY time
                    ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
), normalized AS (
  SELECT *, multiIf(
    startsWith(metric,'cxz.usage.') AND resource['cxz.usage.source']='supervisor',metric,
    startsWith(metric,'cxz.') AND NOT startsWith(metric,'cxz.usage.'),metric,
    resource['cxz.usage.source']!='native','',
    metric='claude_code.cost.usage','cxz.usage.cost_usd',
    metric='claude_code.token.usage' AND attributes['type']='input','cxz.usage.input_tokens',
    metric='claude_code.token.usage' AND attributes['type']='output','cxz.usage.output_tokens',
    metric='claude_code.token.usage' AND attributes['type'] IN ('cacheRead','cache_read'),'cxz.usage.cache_read_tokens',
    metric='claude_code.token.usage' AND attributes['type'] IN ('cacheCreation','cache_creation'),'cxz.usage.cache_write_tokens',
    metric='codex.turn.token_usage' AND attributes['token_type']='input','codex.input_inclusive',
    metric='codex.turn.token_usage' AND attributes['token_type']='cached_input','cxz.usage.cache_read_tokens',
    metric='codex.turn.token_usage' AND attributes['token_type']='cache_write_input','cxz.usage.cache_write_tokens',
    metric='codex.turn.token_usage' AND attributes['token_type']='output','cxz.usage.output_tokens',
    '') AS key
  FROM changes
  WHERE time >= fromUnixTimestamp64Milli({from:Int64}) AND isFinite(delta) AND delta>=0
)
SELECT toString(toDate(time,'UTC')) AS date, resource['cxz.session.id'] AS session,
 sumIf(delta,key='cxz.conversation.events') AS events,
 sumIf(delta,key='cxz.conversation.inputs') AS inputs,
 sumIf(delta,key='cxz.conversation.assistants') AS assistants,
 sumIf(delta,key='cxz.conversation.bytes') AS bytes,
 sumIf(delta,key='cxz.conversation.event_bytes') AS event_bytes,
 sumIf(delta,key='cxz.turn.finished') AS turns,
 greatest(sumIf(delta,key='cxz.turn.token_reported'),toFloat64(countIf(startsWith(key,'cxz.usage.') AND metric IN ('claude_code.token.usage','codex.turn.token_usage'))>0)) AS token_turns,
 greatest(sumIf(delta,key='cxz.turn.cost_reported'),toFloat64(countIf(key='cxz.usage.cost_usd' AND metric='claude_code.cost.usage')>0)) AS cost_turns,
 sumIf(delta,key='cxz.usage.input_tokens') + greatest(0,
   sumIf(delta,key='codex.input_inclusive')-
   sumIf(delta,key='cxz.usage.cache_read_tokens' AND metric='codex.turn.token_usage')-
   sumIf(delta,key='cxz.usage.cache_write_tokens' AND metric='codex.turn.token_usage')) AS input_tokens,
 sumIf(delta,key='cxz.usage.output_tokens') AS output_tokens,
 sumIf(delta,key='cxz.usage.cache_read_tokens') AS cache_read,
 sumIf(delta,key='cxz.usage.cache_write_tokens') AS cache_write,
 sumIf(delta,key='cxz.usage.cost_usd') AS cost
FROM normalized
WHERE key!=''
GROUP BY date,session
ORDER BY date,session
FORMAT JSONEachRow
