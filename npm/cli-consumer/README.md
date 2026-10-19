# `@metorial/cli`

The official npm package for the Metorial consumer CLI (`metorial`).

Use it to sign in to your Metorial portal, connect its integrations, and call MCP tools from your terminal.

```bash
npm install -g @metorial/cli
metorial login
metorial catalog list
metorial setup <catalog-item-id>
metorial list
metorial call <integration-id> <tool> -d '{}'
```

For Magnetar resource administration, raw API requests, and examples, install [`@metorial/admin`](https://www.npmjs.com/package/@metorial/admin).
