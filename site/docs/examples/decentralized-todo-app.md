---
id: decentralized-todo-app
title: Build A Decentralized Todo App using VueJS & Rust (WebAssembly)
sidebar_label: Build A Decentralized Todo App using VueJS & Rust (WebAssembly)
---

_The original tutorial is written by Jonathan Ma and can be found [here](https://medium.com/@jjmace01/build-a-decentralized-todo-app-using-vue-js-rust-webassembly-5381a1895beb)._

One of the first tutorials you come across when learning a new programming language is to build a Todo App. While some think it is reinventing the wheel to build one, one can get a grasp of how the framework/libraries work.

Here is the final working product: https://wavelet-todo.surge.sh, in case you are in a rush :)

This tutorial adds a bit of ***twist*** to the normal Todo App by making it decentralized on the Wavelet blockchain, as a decentralized app (Dapp).

## Why Dapps? 
Why, you might ask, do I have to build a Dapp and store data on the blockchain, when I can just store my data with the conventional database?

Dapps use a blockchain to store data. That data gets spread throughout potentially hundreds or thousands of nodes in a mesh network, so that even if a hacker DDoS’ the majority of your blockchains network, your app would still remain secure and available. (From [Build A Decentralized Chat Using JavaScript & Rust (WebAssembly](examples/decentralized-todo-app.md))

With this in mind, we can build apps that are resilient and secure on the blockchain. As a developer, one only have to worry about code, and less about infrastructure security hardening.

Let's create a decentralized todo list.

## The Frontend
With reference to several existing Todo App tutorials, we will be using using [Vue.js](https://vuejs.org/) and [nes.css](https://nostalgic-css.github.io/NES.css/) to develop our todo Dapp frontend.

If you are not familiar with Vue, you are most welcome to use other frameworks that suit your workflow or infrastructure (e.g. React, Angular etc.).

Here is the basic **scaffold** we will build upon:

**Vue**
```javascript
new Vue({
  el: "#app",
  data: {
    todos: [
      { id: "0", content: "Learn Vue.js", done: false },
      { id: "1", content: "Learn Rust", done: false },
      { id: "2", content: "Play around in Wavelet", done: true },
      { id: "3", content: "Build an awesome Dapp", done: true }
    ]
  },
  methods: {
    addTodo({target}){
    	this.todos.unshift({text: target.value, done: false, id: Date.now()})
      target.value = ''
    },
    removeTodo(id) {
    	this.todos = this.todos.filter(todo => todo.id !== id)
    }
  }
})
```
**HTML**
```html
<link href="https://fonts.googleapis.com/css?family=Press+Start+2P" rel="stylesheet">
<div id="app">
  <h2>Todo List</h2>
  <input class="nes-input" placeholder="Add todo..." v-on:keyup.enter="addTodo"/>
  <div class="spacer"></div>
  <ol>
    <li v-for="todo in todos">
      <label>
        <input type="checkbox"
          class="nes-checkbox"
          v-model="todo.done">

        <del v-if="todo.done">
          {{ todo.content }}
        </del>
        <span v-else>
          {{ todo.content }}
        </span>
        <button class="nes-btn is-error" v-on:click="removeTodo(todo.id)">&times;</button>
      </label>
    </li>
  </ol>
</div>
```
**CSS**
```css
body {
  background: #20262E;
  padding: 20px;
  font-family: "Press Start 2P";
}

#app {
  background: #fff;
  border-radius: 4px;
  padding: 20px;
  transition: all 0.2s;
}

li {
  margin: 8px 0;
}

h2 {
  font-weight: bold;
  margin-bottom: 15px;
}

del {
  color: rgba(0, 0, 0, 0.3);
}

.nes-btn {
  padding: 0px 4px;
  margin: 0px;
}

.spacer {
  height: 1rem;
}
```

We will be adding in code to **link** up this frontend to the backend (via Wavelet smart contracts) in the next section. So don’t move away yet!

## The Backend
Think of the backend for our decentralized todo app compared to the conventional as follows:

Node JS HTTP Server => Rust Smart Contract
Database (MySQL, Postgres, MongoDB etc.) => Wavelet (blockchain)

### Getting some PERLs

First things first, in order to deploy and run smart contracts, you will need gas. You can think of gas as the cost for hosting/server rental, where it is calculated according to the machine CPU and memory consumption of your app. (Note: Wavelet’s testnet is live, however, the fees for invoking smart contracts racking up to 250,000 PERLs is in no way representative of how expensive smart contracts will really be when Wavelet is fully decentralized and publicly open)

You can get PERLs from the testnet faucet on Discord: https://discord.gg/dMYfDPM

### Preparing to Write the Contract

You may refer to the steps [here](examples/decentralized-chat-app.md) for setting up Rust and writing your first Wavelet smart contract.

### Writing the Contract

As we are writing a Todo Dapp, we have several additional functions implemented compared to the Decentralized Chat app. We would need the following features:

1. Removing a Todo
2. Toggling the ‘done’ status for each Todo

Here we have the full smart contract written out as the following:

```rust
use std::error::Error;

use smart_contract_macros::smart_contract;

use smart_contract::log;
use smart_contract::payload::Parameters;
use std::collections::VecDeque;

struct Todo {
    content: String,
    done: bool
}

struct TodoList {
    logs: VecDeque<Todo>
}

const MAX_LOG_CAPACITY: usize = 20;
const MAX_MESSAGE_SIZE: usize = 250;

fn prune_old_todos(list: &mut TodoList) {
    if list.logs.len() > MAX_LOG_CAPACITY {
        list.logs.pop_front();
    }
}

#[smart_contract]
impl TodoList {
    fn init(_params: &mut Parameters) -> Self {
        Self { logs: VecDeque::new() }
    }

    fn add_todo(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
        let todo = Todo { content: params.read(), done: false };

        // Ensure that todo contents are not empty.
        if todo.content.len() == 0 {
            return Err("Message must not be empty.".into());
        }

        // Ensure that content are at most 240 characters.
        if todo.content.len() > MAX_MESSAGE_SIZE {
            return Err(format!("Message must not be more than {} characters.", MAX_MESSAGE_SIZE).into());
        }

        // Push todo into list.
        self.logs.push_back(todo);

        // Prune old todos if necessary.
        prune_old_todos(self);

        Ok(())
    }

    fn remove_todo(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
        let target:usize = params.read();
        if target < self.logs.len() {
            self.logs.remove(target);
        }
        else {
            return Err("Index out of bounds.".into());
        }
        Ok(())
    }

    fn toggle_todo(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
        let target:usize = params.read();
        if target < self.logs.len() {
            let target_ref = &mut self.logs[target];
            target_ref.done = !target_ref.done;
        }
        else {
            return Err("Index out of bounds.".into());
        }
        Ok(())
    }

    fn get_todos(&mut self, _params: &mut Parameters) -> Result<(), Box<dyn Error>> {
        let mut todos = Vec::new();

        for todo in &self.logs {
            todos.insert(0, format!("<{}> {}", todo.content, todo.done));
        }

        log(&todos.join("\n"));

        Ok(())
    }
}
```
89 lines of code. Small, eh? To explain what’s going on here:

The smart contract exposes four functions that may be called by our frontend: **get_todos()**, **add_todo(content: String)**, **toggle_todo(id: uint32)** and **remove_todo(id: uint32)**.

The **String** param for **add_todo(…)** is read using the **param.read()function** when declaring the todo variable. For more details on how inputs are specified into smart contracts, click [here](smart-contracts.md#parsing-input-parameters).

Todos are stored in a [VecDeque](https://doc.rust-lang.org/std/collections/struct.VecDeque.html?source=post_page---------------------------) (a double-ended queue where you can push items to the front of the queue, or to the end of the queue), which is initialized in init().

Each todo contains a content field.

You call **add_todo(…)** to place a todo into the logs queue. Empty todo descriptions are not allowed, and todos may at most be 250 characters.

The logs queue has a capacity of at *most 20 todo entries*. Should the queue be full when a new chat log is to be inserted into the queue, the queue removes the oldest todo using **logs.pop_front()**. This logic is handled in the **prune_old_todos()** function.

You may call **get_todos()** to get a human-friendly readable version of a chatlog. Sender ID’s get printed out as a hex string, and are concatenated with the senders message contents.

Then a **log!()** macro is used to have the smart contract provide to your frontend all available chat messages from **get_todos()**.

You call **toggle_todo(…)** to toggle the done boolean variable which indicates whether you can completed the todo.

You can also call **remove_todo(…)** to remove the todo from the Todo queue.

Let’s now build our smart contract using Rust’s Cargo tool:

```shell
❯ cargo build --release --target wasm32-unknown-unknown
``` 

… and if everything goes well, in the `target/wasm32-unknown-unknown/release` folder, you will see a `chat.wasm` file.

That is your smart contract, ready to be deployed on Wavelet’s blockchain.

### Deploying the contract

You may refer to the steps in our [previous tutorial](examples/decentralized-chat-app.md) for deploying the contract you just wrote on the Lens platform. You can test out the functions you wrote on the Lens platform first, before integrating it to the frontend. Make sure to replenish your wallet with PERLs after invoking calls!

## Linking up the Frontend and Backend via Wavelet JS Client

In this section we will provide you with the glue needed to stitch the frontend and backend together.

Building on top of the scaffold we had in the previous section, we need to make a few changes to the main.js and App.vue files.

**main.js**
```javascript
import {Wavelet,Contract} from "wavelet-client";

const client = new Wavelet('https://testnet.perlin.net');
const wallet = Wavelet.loadWalletFromPrivateKey("<YOUR PRIVATE KEY>");
const contract = new Contract(client, "<YOUR CONTRACT ID>");

Vue.use({
  install (Vue) {
    Vue.prototype.$contract = contract
    Vue.prototype.$wallet = wallet
    Vue.prototype.$client = client
  }
})
```

**App.vue**
```html
<template>
  <div id="app">
    <h2>Todo Dapp, written in JavaScript + Rust (WebAssembly)</h2>
    <p>Powered by <a href="https://wavelet.perlin.net">Wavelet</a>. Click <a href="https://medium.com/perlin-network/build-a-decentralized-chat-using-javascript-rust-webassembly-c775f8484b52">here</a> to learn how it works, and <a href="https://github.com/perlin-network/decentralized-chat">here</a> for the source code. Join our <a href="https://discord.gg/dMYfDPM">Discord</a> to get PERLs.</p>
    <input class="nes-input" placeholder="Add todo..." v-on:keyup.enter="addTodo"/>
    <div class="spacer"></div>
    <ol>
      <li v-for="(todo, idx) in todos">
        <label>
          <input type="checkbox"
            class="nes-checkbox"
            @click="toggleTodo(idx)"
            :checked="todo.done">

          <del v-if="todo.done">
            {{ todo.content }}
          </del>
          <span v-else>
            {{ todo.content }}
          </span>
          <button class="nes-btn is-error" v-on:click="removeTodo(idx)">&times;</button>
        </label>
      </li>
    </ol>
    <hr/>
    <h3>Logs</h3>
    <textarea readonly>{{ log.join("\n") }}</textarea>
  </div>
</template>

<script>
import Vue from 'vue';
import JSBI from "jsbi";
import {Wavelet,Contract} from "wavelet-client";
const BigInt = JSBI.BigInt;
export default {
  name: 'app',
  mounted() {
    this.init();
  },
  data() {
    return {
      wallet: null,
      contract: null,
      todos: [],
      log: []
    }
  },
  methods: {
    async init() {
      var self = this
      return await this.$contract.init().then(resp => {
        self.getTodos()
        self.listen()
      })
    },
    async listen() {
      var self = this
      await this.$client.pollConsensus({
        onRoundEnded: msg => {
          (async () => {
              await self.$contract.fetchAndPopulateMemoryPages();
                self.getTodos();
          })();
        }
      });
    },
    getTodos() {
      var raw = this.$contract.test('get_todos', BigInt(0));
      this.todos = raw.logs[0].split('\n').reverse().map((a, aidx) => {
        var matched = a.split(' ');
        return {
          id: aidx,
          content: matched[0].replace(/[\<\>]/g, ''),
          done: eval(matched[1])
        }
      });
    },
    async addTodo({target}) {
      var self = this
      return await this.$contract.call(
        this.$wallet, 
        'add_todo', 
        BigInt(0), 
        BigInt(250000),
        {type: "string", value: target.value},
      ).then(resp => {
        target.value = '';
        self.log.push(resp.tx_id);
      })
    },
    async removeTodo(id) {
      var self = this
      return await this.$contract.call(
        this.$wallet, 
        'remove_todo', 
        BigInt(0), 
        BigInt(250000),
        {type: "uint32", value: id},
      ).then(resp => {
        self.log.push(resp.tx_id);
      })
    },
    async toggleTodo(id) {
      var self = this
      return await this.$contract.call(
        this.$wallet, 
        'toggle_todo', 
        BigInt(0), 
        BigInt(250000),
        {type: "uint32", value: id},
      ).then(resp => {
        self.log.push(resp.tx_id);
      })
    }
  }
}
</script>

<style>
html, body, pre, code, kbd, samp {
  font-family: "Press Start 2P";
}
body {
  background: #20262E;
  padding: 20px;
}
#app {
  background: #fff;
  border-radius: 4px;
  padding: 20px;
  transition: all 0.2s;
  margin: 2rem;
  padding: 2rem;
}
ol {
  margin-left: 16px;
}
li {
  margin: 8px 0;
}
h2 {
  font-weight: bold;
  margin-bottom: 15px;
}
del {
  color: rgba(0, 0, 0, 0.3);
}
.nes-btn {
  padding: 0px 4px;
  margin: 0px;
}
.spacer {
  height: 1rem;
}
textarea {
  width: 100%;
  border: none;
  height: 200px;
}
</style>
```

Note that in order to make the todos update after they are added/removed/toggled, you need to poll for updates (in Wavelet, that is consensus changes). Add the following inside the load() method:

```javascript
await client.pollConsensus({
    onRoundEnded: msg => {
        (async () => {
            await contract.fetchAndPopulateMemoryPages();
            console.log("Chat logs updated:", contract.test('get_messages', BigInt(0)));
        })();
    }
});
```

## Conclusion

Up till this point, you have built your very own decentralized Todo Dapp, with an awesome and even awesomer (is there a word like this?) backend! Give yourself a round of applause!

Here how it looks:
![alt text](https://miro.medium.com/max/700/1*8gg3do_CISV9pVd4MZHdjQ.png "Decentralized todo app frontend")

Congratulations, you just made your very first, scalable, secure Wavelet Dapp!

There is a lot more to explore, such as a plethora of additional functionalities provided by the Wavelet JS client, alongside the extensive set of documentation for Wavelet located on this site.

Source code for the full tutorial is available here: https://github.com/johnhorsema/wavelet-todo
