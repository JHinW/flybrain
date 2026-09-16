var path = require('path');
var dbModule = require('../server/db');
var ai = require('../server/ai');

var db = dbModule.openDb();
var config = ai.getConfig();
var limit = parseInt(process.argv[2], 10) || 20;

var history = db.getChatHistory(limit);

if (history.length === 0) {
  console.log('No chat interactions recorded yet.');
  db.close();
  process.exit(0);
}

console.log('=== Recent AI Chat Interactions (last ' + history.length + ') ===\n');

for (var i = 0; i < history.length; i++) {
  var item = history[i];
  var time = item.timestamp ? item.timestamp.replace('T', ' ').replace('Z', ' UTC') : '';
  if (item.role === 'user') {
    console.log('\x1b[36m[' + time + '] User:\x1b[0m');
    console.log('  ' + item.message + '\n');
  } else {
    console.log('\x1b[32m[' + time + '] Assistant (' + config.chatModel + '):\x1b[0m');
    console.log('  ' + item.message + '\n');
  }
}

db.close();
