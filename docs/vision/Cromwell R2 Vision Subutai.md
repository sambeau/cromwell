# Subutai 

Subutai (the new name for *Cromwell*) is a planning, workflow and orchestration system for small teams building large software projects using AI Agents.

It manages the process from original concept through to individual tasks, using familiar planning concepts: project, roadmap, milestone, plan (which we call an initiative, so as not to confuse with development plan); and familiar development concepts: development plan, feature, task.

## Discussion document

This is a **design discussion document**. The intention is to use this to create a draft design document and, through discussion and iteration: a final design.

Subutai builds on top of the work of Cromwell. Consider it Cromwell *revision 2* (not version 2). 

It is a clarification of the vision rather than a whole new project. All the fundamentals of the vision of Cromwell (and the lessons learned from Kanbanzai) are untouched, especially those gleaned from modern up-to-date research and the ’10-principals’. 

The core of Cromwell is not changed by this document. This is a clarification of the workflow and the corresponding UI, mostly around what the role of AI Chat agents are versus agentic API agents and any corresponding changes to the tools and web user interface needed to support these changes.

There will also be a section on refinements to the web UI based on the recent manual smoke testing.

We will start the discussion with a reiteration of what the core vision is.

## Teams

The small teams building the software are a mixture of humans and AI Agents. The team has a natural decision into two sub-teams: product and development.

The product team is made up of humans and AI Agents in the roles of product manager, designer, and project manager.

The roles are usually staffed as follows:
- Product manager: human
- Designer: human and AI chat agent, working together
- Project manager: AI chat agent with guidance from human

## Roles

The development team is made up of a mixture of API-based AI Agents with various roles, skills, and agentic capabilities (thinking focused vs coding focused) and software tools. Roles include orchestrator, implementor, reviewer, planner, estimator

- **Project manager:** AI chat agent that bridges the world of product and development
- **Orchestrator:** a deterministic MCP server that manages the AI Agents through their respective APIs (the heart of Subutai)
- **Orchestrated AI Agents:** a set of API agents each with an associated role, skill, set of permitted tools. Each AI Agent can have a different API to suit their level of thinking and focus. 

Orchestrated AI Agents will include:
- **Planner:** a high-thinking AI Agent role with skill of reading specifications, planning, decomposition, parallel task planning etc
- **Implementor:** code-optimised AI roles with specialist skills, e.g. Go, Javascript, Typescript, React, React Native etc.
- **Code-reviewer:** specialist code reviewers to match each implementor
- **Development plan reviewer:** A planning role with similar skills to a Planner but specialising in reviewing development plans.
- **Estimator:** a high-thinking AI Agent role that can read a development plan and create reliable estimates (perhaps with the aid of tools)

The goal of the product team is to create two documents per piece of work (initiative or feature): a high-quality design document aimed at human readers, and an accurate specification document aimed at agentic readers. 

The goal of the development team is to turn a specification into high quality software — or more formally to take a piece of development work through the process until it meets the **Definition of Done** (DoD).

## Ad-hoc workflow vs Orchestration

Our research made it clear that deterministic work should be managed by code.

Development work *is deterministic:* it has a strict order, gates, cycles and finally a DoD that must be met to be called complete. It is best managed by an orchestration tool: software with rules baked in.

Design work *is not deterministic* and is, by its nature, a slow, messy process that might require a number of iterations (multiple drafts) before reaching a final approved design. Similarly an initiative may need to be broken into sub-initiatives and features, all requiring their own design documents. It is best managed by humans and AI Agents chatting with each other until a final design is created.

### AI Agents

The AI Agent being chatted to during design and specification needs to be a model with a high-level of capability. In my case it will generally be a high-thinking-level Claude model: Fable or Opus.

The orchestrated agents need to be accessed through an API, but can be of any level or provider. We should aim to allow each role to be assigned an API and effort level suited to its tasks. In my case the orchestrated agents are likely to be deepseek-v4-flash or Claude Sonnet. However, they are unlikely to be the high-thinking *expensive* models used for project planning and design.

### Chat AI is fundamental to the workflow

This is an important point: there is a division of AI models between product planning and design *and* orchestrated development: higher quality, more expensive agents will be used to create the plans, designs, feature decomposition, and specs than will be used to implement the code.

Just as important: the way AI Agents are paid for, accessing some models, in particular high-thinking Claude models, is incredibly expensive through the API. Conversely using a time-boxed ‘pro’ plan is reasonable affordable, even for the most expensive models.

Thus, the natural division between the parts where a human chats with an agent vs where an orchestrator manages development, will also be a fundamental division of AI agents and their payment models.

- Product planning, design, specification -> Chat-based
- Development Planning, review, implementation, review, testing, DoD approval, etc -> API based 

## Documents

Subutai is **document-based**, in that the main interaction between the product team and the development team is through documents, in particular **specification documents**. However documents are used throughout the workflow:

- The main design document can be supported by various supplemental documents along with UI designs, icons, logos etc.
- Each review may generate a review document
- Decisions may be recorded in decision documents
- Various other documents can also be stored in the system, including:
	- Test reports
	- Research reports
	- Code quality reports
	- Security reports
	- Notes, todo lists etc.

While Subutai will accept (and track) documents from anywhere with a repository, documents created through the Subutai tools (MCP server or UI) will apply a standard file organisation system, including giving each document with a designated role in the workflow an assigned, deterministic name with a unique ID.

Subutai is a **specification-led** (or, more rightly, specification-centred) system in that the system hands over from human to AI developer through a specification: a definition of what will be built that acts like a contract between human and AI.

## Workflow

The workflow system has two phases: one human-driven (with AI support), the other AI-driven, with human support.

- PLANNING: human-driven workflow for planning
- DEVELOPING: AI-driven workflow for developing

The pivot-point is the specification:

Planning -> *specification* -> developing

To get a group of features from idea to specification the system uses:

- **Project** - the parent object of the project
- **Initiative** - a plan to do some work
- **Milestone** - an unordered list of work (that we call *deliverables*)
- **Roadmap** - an ordered list of *milestones*
- **Checklist** - an unordered list of *jobs* 
- **Job** - A manual task for a human. 

## State

State is metadata recorded by the system about documents and initiatives that maps the flow of work through planning, approval, implementation and completion.

The system uses two types of state:

1. **Document state**: documents are normal project files managed by Git. Document state is metadata about documents tracked by the system e.g. completion and approval status.
2. **Workflow state**: is the state of each piece of work being tracked (initiatives and deliverables), plus other data useful for keeping track of each initiative: estimates, tokens used, logs approval, readiness etc.

Workflow state its managed by the Subutai MCP server and stored in an SQL database.

The Subutai MCP server provides tools for AI Chat Agents to manage state on an ad-hoc basis (designing, planning, specifying, fixing, fudging), an orchestrator to manage workflows in a strict deterministic way, 

## Formal Definitions:

### Work Entities

These a where *work* is **defined**:

A **Project**:

- Conceptually, the top-level initiative
- Can contain child initiative
- Has no parent initiative

An **Initiative**:

- A unit of conceptual work
- Can have roadmaps and milestones
- Can have child initiatives

A **Job**:

- A task that a human needs to perform, usually admin tasks e.g. sign up for an account, find an API key, create a graphic design, choose an icon etc.
- Represented by an entry on a **checklist**
- When a human completes task, item is manually ticked.
- Every box must be ticked for the checklist to be complete and contribute to the completion of a milestone

### Progress Indicator Entities

Progress Indicator Entities collate progress data from work entities.

These a where *work* is **tracked**:

A **Roadmap**:

- A roadmap is an *ordered* collection of **deliverables**

A **Milestone**:

- A milestone is an *unordered* collection of **deliverables**

A **Checklist**:

- A checklist is an *unordered* collection of **jobs**

A **Deliverable**:

- A deliverable is an **initiative** *or* a **milestone** *or* a checklist.

## Skills and Roles

If we remove the need for a design reviewer we can remove the associated role and skill.

We might need to tighten up the roles we have based on the research from kanbanzai. I notice that they are not consistent in form.

## Agents’ UI: MCP sever tools

An agentic chat-based UI needs to be able to manage workflow on an ad-hoc basis, so requires a full suite of MCP tools to manipulate state with as much determinism as possible inbuilt to prevent broken state entering the system.

The change between Cromwell and Subutai is a refocussing on Chat as a fundamental part of human workflow.

A human should be able to approve a document by pressing a button in the UI or telling an AI Chat agent that a document is approved.

Orchestration will be initiated by pressing a button in the UI to ensure that the AI Agent does not attempt to do a ‘quick fix’ or orchestrate the work themselves.

## Humans’ UI: Web-based UI

In addition to Chat, humans require a web-based UI to see a visual representation of initiatives, track progress, manage documentation logically grouped, approve status etc. What we have at present in Cromwell seems to be working well (albeit only being for initiatives and features). We may meed to iterate and polish it but fundamentally, it’s working. So no need to change this at the moment.

Tracking the progress of implementation is less successful. In normal operation it’s less of a concern, though can be useful, especially for debugging purposes (for instance, there is no way from the UI to work out why estimations are so different to reality). The recent activity log is a good start but it isn’t clear enough for tracking the major transitions of development, while lacking the detail of what is actually happening with AI agents to be able to review and debug what they are doing.

I would like to discuss options here. For instance, would it be possible to keep a complete log of the whole conversation with the agent, like you can get with other tools?

Similarly, seeing more of a summary of major milestones in the development process would be clearer and have less noise. “Reviewing Spec”, “Spec passed”, “Implementing”, “Testing”, “Reviewing implementation” etc. 

### Creating a new initiative

*(Or sub-initiative, or feature etc.)*

If we are to keep naming conventions consistent, we should add a way to create new initiatives etc **with their respective documents**. 

For instance, creating a new initiative should also create a placeholder document to go with it—in the UI and in a predefined place in the project files.

The current UI feels rather clunky when you have to type a path into a text-box to join a file to the initiative. It would be much nicer for the system to set up the first document and manage it for you on disk—not to replace the possibility of starting with a file and attaching it (or a Chat agent writing first daft and creating an initiative for it), but as the more common route.

This would imply a standard way of managing (and naming) files.

### Editing

## Git

There is no change to how Git is used from Cromwell.

A workflow management system with MCP server tools does not negate the need for Git and its associated workflows. Git continues to manage the project’s code and documents.

Subutai manages the metadata state not suited for Git (as discovered, the hard way, with kanbanzai).

## Names and IDs

Cromwell didn’t enforce any naming conventions or IDs on documents, designs or initiatives. However, as previous projects have shown, having IDs makes managing documents (grouping, search, discussion) far easier. Subsequently, in a distributed project with multiple actors, is important to manage IDs to prevent clashes.

We need a simple, foolproof system of document names with IDs where, as much as possible, we match IDs between related documents to allow them to easily be visually grouped.

We should discuss this, and perhaps look back at Kanbanzai which went through various iterations before settling on a system.

While having a web UI makes it easier to manage documents, the nature of being able to edit them directly on disk means that we should attempt to make.

## Document grouped by type or initiative?

This project groups files by type: designs live in a folder and are all prefixed with `DESIGN-`.

Kanbanzai moved to documents grouped by initiative, so all files for one initiative would live in the same folder.

Humans find the latter much easier (assuming the folder has an easy-to-understand folder-name).

A web UI, can make managing document files much easier by:
- listing all the files associated with an initiative
- supplying a way to copy a path
- supplying a way to open a file (or folder) in the filesystem 
- providing a create-file button within an initiative’s page that asks for type and subject with which to generate a file with a standard name in a standard folder
- providing a built-in edit function

We should discuss how easy it would be to provide in-UI editing of documents.

### Editing text documents from the UI

I would like to look into whether it would be possible to provide editing within the browser for documents.

What are the complications, if any? How would it work with Git? Could save also commit?

Ideally the Subutai web UI would have all the normal functions of a web-based workflow UI: CRUD, approve etc

I think this would make the management f documents far easier for the humans, while, by keeping files on disk, it would also make it possible for AI agents to get easy access to them (plus Git could still manage them).

Is there a complexity here that I am missing?

## BUGs

We need to discuss BUGs, something that Cromwell never got to. The consensus from Kanbazai is that BUGs should be treated as a sub-type of feature, and, usually attached to the initiative that created the bug.

It is important that agents should be able to record bugs without human intervention. However, humans should always be involved in the approval of a bug and its proposed fix.

Humans should be able to report BUGs to an AI Chat acting as project manager, or create bug reports in the UI.

We should come up with a plan for them.

## DECISIONS

It is important to record decisions as Agentic Memory purposes. This project uses decisions (`DEC-001` – `DEC-005`). 

I am not sure what role they would perform in the Workflow system (and whether they would be attached to initiatives?) but it seems important to record them and to provide a viewer for them. At the very least Subutai should manage the IDs of them to prevent clashes.

We should come up with a plan for them.

## CONCLUSION

- This is the start of a discussion about a revised Cromwell (with a new name), not a version 2 Cromwell
- I would like your considered opinions
- The goal is a draft design document