var fs = require('fs');
var path = require('path');
var ai = require('../server/ai');

var policyPath = path.join(__dirname, 'caretaker-policy.md');

function parseArgs() {
  var args = process.argv.slice(2);
  var prompt = '';
  for (var i = 0; i < args.length; i++) {
    if (args[i] === '--policy' && args[i + 1]) {
      policyPath = args[i + 1];
      i++;
    } else if (args[i] === '--prompt' && args[i + 1]) {
      prompt = args[i + 1];
      i++;
    } else if (!args[i].startsWith('--')) {
      prompt = args.slice(i).join(' ');
      break;
    }
  }
  return prompt;
}

function cleanJsonText(rawText) {
  if (!rawText || typeof rawText !== 'string') return '';
  var text = rawText.trim();
  // Strip markdown ```json ... ``` or ``` ... ``` wrappers if present
  var match = text.match(/```(?:json)?\s*([\s\S]*?)\s*```/);
  if (match) {
    text = match[1].trim();
  }
  return text;
}

async function runDecide(prompt) {
  var policyContent = fs.readFileSync(policyPath, 'utf8');
  var client = ai.getClient();
  var config = ai.getConfig();
  var model = config.agentModel;

  var response = await client.messages.create({
    model: model,
    max_tokens: config.maxTokens,
    system: policyContent,
    messages: [{ role: 'user', content: prompt }]
  });

  var text = ai.extractText(response);

  var jsonStr = cleanJsonText(text);
  // Verify it is valid JSON
  JSON.parse(jsonStr);
  return jsonStr;
}

function main() {
  var prompt = parseArgs();
  if (prompt) {
    execute(prompt);
  } else {
    var stdinData = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', function(chunk) { stdinData += chunk; });
    process.stdin.on('end', function() {
      if (!stdinData.trim()) {
        process.stderr.write('[decide] Error: Empty prompt received\n');
        process.exit(1);
      }
      execute(stdinData.trim());
    });
  }
}

function execute(prompt) {
  runDecide(prompt)
    .then(function(jsonOutput) {
      process.stdout.write(jsonOutput + '\n');
      process.exit(0);
    })
    .catch(function(err) {
      process.stderr.write('[decide] Error evaluating policy: ' + err.message + '\n');
      process.exit(1);
    });
}

if (require.main === module) {
  main();
}

module.exports = {
  cleanJsonText: cleanJsonText,
  runDecide: runDecide
};
