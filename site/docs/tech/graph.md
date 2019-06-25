---
id: graph
title: Graph
sidebar_label: Graph
---

#### General information
Graph (`graph.go`) is the module which holds DAG implementation used in the Wavelet.

`Graph` is main structure which holds all DAG related information and implements all the functionality.
All public/exported API is concurrently safe.

`Graph` stores transactions in couple of different indices - in maps for quick access and some in binary trees for quick search (e.g. parents and critical transactions).

#### Adding transactions
There are 3 reasons why transaction won't be added and action will result with error:
- transaction already exists in the graph
- depth of the transaction is below threshold (smaller than graph's root depth - `MaxDepthDiff`) - which basically means that transaction is too old for current graph's state
- transaction is not valid (see [Transaction validation](#transaction-validation))

If none of the above is true, then transaction will be added to one of the indices, which won't affect graph structure, call it temporary transactions.
In order for transaction to be part of the graph it should be "complete" - graph should have all of the ancestors of the transaction up to some depth (`MaxParentsPerTransaction` rounds difference).
All of the missing or incomplete ancestors (up to depth limit) will be stored as missing transactions (see downloading transactions). And for those which present in the graph, they will be stored in `children` index.
If at least one of the ancestors is missing, transaction will be stored in `incomplete` index and error will be returned.
And otherwise transaction will be added to the graph.

##### Updating graph with transaction
Firstly all of the transaction parents are validated (see [Transaction parents validation](#transaction-parents-validation))
If any is not valid - transaction and its children deleted from all indices of the graph.
Graph height may be updated if new transaction's depth is bigger.
After that transaction will be added to search indexes (b-trees for parent and critical transactions). And for all existing and incomplete children of the transaction we need to check other parents for presence and completion. This is needed because new transaction may be the only one which is preventing child transaction from being complete. 
In such case child transaction will be marked as complete and added to the graph as well.

#### Transaction validation
Here are conditions which needs to be met in order for transaction to be valid (for each case respective error will be returned if its not satisfied):
- it should not have zero/empty id (see transaction id)
- it should not have zero/empty sender and creator
- it should have at least one and at most `MaxParentsPerTransaction` parents
- it should not include itself in the parents, parents should be lexicographically sorted and there should be no duplicated parents
- it should have known tag - if its nop transaction it should have no payload, otherwise there should be non empty payload
- sender and creator (if they aren't the same) should have correct signatures

##### Transaction parents validation
First of all if transaction's depth is below root depth by `MaxDepthDiff` rounds we do not check its parents at all.
Then if there is a parent with depth lower than transaction one by `MaxDepthDiff` round, such parent is considered invalid and error is returned.
If transaction's depth is not bigger exactly by one than max depth of parents - then such parents considered invalid. 
In the opposite case - transaction considered as the one which has valid parents.

#### Parents selection
As was mentioned in the begining, potential parents transaction are stored in separate index - binary tree in particular. Transaction are sorted by depth and in case of equal depth - by seed length (see transaction seed).
Tree is traversed in descending order, i.e. higher depth goes first. First `MaxParentsPerTransaction` number of eligible parents will be selected as parents for new transaction. If parent has depth lower than graph by `MaxDepthDiff` rounds or has children which present in the graph and "complete" such parent won't be taken and instead will be removed from parents index in the graph.
In other words we prioritise "leaf" transactions to be parents.  

#### Critical transaction selection
Potential critical transactions (see critical transaction) stored in separate index in form of binary tree, same way as eligible parent transactions - sorted by depth first and seed length second.
Tree is traversed in ascending order, i.e. lower depth and higher seed len first. First critical transaction with depth higher than root's will be taken. Meanwhile if there will be found transactions during traversal which have depth lower or equal than root's or which aren't critical, they will be removed from this index.