# Ground — Adjudicated Axioms (v2)

These are claims pinned at seed time as adjudicated `true` (groundedness 1.0) or `false` (0.0). They are bedrock that the rest of the graph builds on. Adjudicated claims are skipped during EigenTrust iteration — the algorithm cannot move them.

In v2, **every adjudicated claim must carry ≥2 citations to anchored sources, each with a verbatim quote**. Citations on adjudicated claims are still subject to the mechanical containment check; FACTS.md is the source of truth for the propositions and the *intended* citations, but the loader (`ground bootstrap-axioms`) verifies each quote against the cached source body before persisting.

## Anchor tier requirement

Citations on adjudicated claims must come from **Tier 1 anchored sources**: peer-reviewed top journals, primary government datasets, named encyclopedias for definitional claims, or the original publication of a proven theorem. Wikipedia is allowed for definitional claims only, and only as a secondary citation alongside a Tier 1 primary.

## Format

Each axiom carries:
- An ID (stable across migrations)
- A proposition (the claim text)
- An adjudication (`TRUE` or `FALSE`)
- A list of citations: `[source_url, verbatim_quote, polarity, locator]`
- A list of topic anchors (which topic slugs this claim is load-bearing for)

The `bootstrap-axioms` command parses this file, ingests sources, runs mechanical checks, and persists the adjudicated claims. If a citation fails mechanical check at bootstrap time, the loader logs the failure and skips that citation; if a claim ends up with <1 valid citation, the claim itself is skipped and a warning is emitted.

---

## Mathematics

### MATH-01: Gödel's First Incompleteness Theorem

**Proposition**: Any consistent formal system capable of expressing basic arithmetic contains statements that are true but unprovable within the system.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/G%C3%B6del%27s_incompleteness_theorems` — *"Any consistent formal system F within which a certain amount of elementary arithmetic can be carried out is incomplete; i.e. there are statements of the language of F which can neither be proved nor disproved in F."* (supports)
- Original publication: Gödel, K. (1931). "Über formal unentscheidbare Sätze der Principia Mathematica und verwandter Systeme I" — citation pending source ingestion of digitized original or translation in van Heijenoort (1967)

**Topic anchors**: what-incompleteness-actually-means, the-most-important-open-problem-in-computer-science, the-ontological-status-of-mathematical-objects

### MATH-02: Gödel's Second Incompleteness Theorem

**Proposition**: No consistent formal system capable of expressing basic arithmetic can prove its own consistency.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/G%C3%B6del%27s_incompleteness_theorems` — *"the consistency of F cannot be proved in F itself"* (supports)

**Topic anchors**: what-incompleteness-actually-means, the-ontological-status-of-mathematical-objects

### MATH-03: The Halting Problem Is Undecidable

**Proposition**: No general algorithm can determine whether an arbitrary Turing machine halts on a given input.

**Adjudication**: TRUE

**Citations**:
- Turing, A. M. (1937). "On Computable Numbers, with an Application to the Entscheidungsproblem" — citation pending ingestion (digitized version available via `https://www.cs.virginia.edu/~robins/Turing_Paper_1936.pdf`)
- `https://en.wikipedia.org/wiki/Halting_problem` — *"Alan Turing proved in 1936 that the halting problem is undecidable, meaning that no general algorithm exists that can correctly solve the problem for all possible program–input pairs."* (supports)

**Topic anchors**: the-most-important-open-problem-in-computer-science, what-incompleteness-actually-means, capabilities-and-limits-of-quantum-computation

### MATH-04: Cook–Levin Theorem

**Proposition**: Boolean satisfiability (SAT) is NP-complete.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Cook%E2%80%93Levin_theorem` — *"In computational complexity theory, the Cook–Levin theorem, also known as Cook's theorem, states that the Boolean satisfiability problem is NP-complete."* (supports)

**Topic anchors**: the-most-important-open-problem-in-computer-science

### MATH-05: P ⊆ NP

**Proposition**: Every decision problem solvable in polynomial time is also verifiable in polynomial time.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/P_versus_NP_problem` — *"Clearly, P ⊆ NP."* (supports)

**Topic anchors**: the-most-important-open-problem-in-computer-science

### MATH-06: Bayes' Theorem

**Proposition**: For events A and B with P(B) > 0, P(A|B) = P(B|A)P(A)/P(B) is a valid theorem of probability theory.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Bayes%27_theorem` — *"gives a mathematical rule for inverting conditional probabilities, allowing the probability of a cause to be found given its effect."* (supports)

**Topic anchors**: frameworks-for-reasoning-under-uncertainty, sciences-self-correction-problem

### MATH-07: Universal Turing Machines

**Proposition**: A universal Turing machine can simulate any other Turing machine given its description as input.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Universal_Turing_machine` — *"universal Turing machine (UTM) is a Turing machine capable of computing any computable sequence"* (supports)

**Topic anchors**: are-we-living-in-a-simulation, can-syntax-produce-semantics, what-incompleteness-actually-means

---

## Physics

### PHYS-01: Bell Inequality Violations

**Proposition**: Experiments have confirmed violations of Bell inequalities with sufficient rigor to rule out all local hidden variable theories.

**Adjudication**: TRUE

**Citations**:
- Hensen et al. (2015), "Loophole-free Bell inequality violation using electron spins separated by 1.3 kilometres" — DOI 10.1038/nature15759, ingestion target
- `https://www.nobelprize.org/prizes/physics/2022/summary/` — *"for experiments with entangled photons, establishing the violation of Bell inequalities and  pioneering quantum information science"* (supports)

**Topic anchors**: capabilities-and-limits-of-quantum-computation, are-we-living-in-a-simulation

### PHYS-02: No-Communication Theorem

**Proposition**: Quantum entanglement cannot be used to transmit information faster than the speed of light.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/No-communication_theorem` — *"It asserts that during the measurement of an entangled quantum state, it is impossible for one observer to transmit information to another observer, regardless of their spatial separation."* (supports)

**Topic anchors**: capabilities-and-limits-of-quantum-computation

### PHYS-03: Landauer's Principle (Experimental Verification)

**Proposition**: Erasing one bit of information dissipates at least kT ln 2 of energy, and this minimum has been experimentally approached.

**Adjudication**: TRUE

**Citations**:
- Bérut et al. (2012), Nature 483, 187–189 — DOI 10.1038/nature10872, ingestion target. Expected quote: *"the heat dissipated during a logically irreversible memory erasure procedure"*
- `https://en.wikipedia.org/wiki/Landauer%27s_principle` — *"the minimum energy needed to erase one bit of information is proportional to the temperature at which the system is operating"* (supports)

**Topic anchors**: energy-costs-of-information-processing, are-we-living-in-a-simulation

### PHYS-04: Second Law of Thermodynamics

**Proposition**: The total entropy of an isolated system does not spontaneously decrease over time.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Second_law_of_thermodynamics` — *"the entropy of isolated systems left to spontaneous evolution cannot decrease, as they always tend toward a state of thermodynamic equilibrium where the entropy is highest at the given internal energy"* (supports)

**Topic anchors**: energy-costs-of-information-processing, the-physics-of-warming

### PHYS-05: Galaxy Rotation Curves

**Proposition**: Observed rotation velocities of stars in spiral galaxies are significantly higher than predicted by Newtonian gravity applied to visible matter alone.

**Adjudication**: TRUE

**Citations**:
- Rubin & Ford (1970), Astrophysical Journal 159, 379 — ingestion target via NASA ADS
- `https://en.wikipedia.org/wiki/Galaxy_rotation_curve` — *"Considering their mass distributions, the rotational/orbital speeds of galaxies/stars would not be expected to follow rules such as Kepler's third law applying to smaller orbital systems such as stars/planets and planets/moons with most mass at the centre."* (supports)

**Topic anchors**: the-missing-mass-problem

### PHYS-06: Bullet Cluster Mass-Light Separation

**Proposition**: Gravitational lensing observations of the Bullet Cluster (1E 0657-558) show that the center of gravitational lensing is spatially offset from the center of X-ray emission (baryonic matter).

**Adjudication**: TRUE

**Citations**:
- Clowe et al. (2006), ApJ 648 L109 — DOI 10.1086/508162, ingestion target
- `https://en.wikipedia.org/wiki/Bullet_Cluster` — *"lensing studies of the Bullet Cluster are claimed to provide strong evidence for the existence of dark matter."* (supports)

**Topic anchors**: the-missing-mass-problem

---

## Neuroscience

### NEURO-01: Neural Correlates of Consciousness

**Proposition**: Specific patterns of brain activity reliably correlate with specific conscious experiences, as measured by fMRI, EEG, and single-neuron recording.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Neural_correlates_of_consciousness` — *"The neural correlates of consciousness (NCC) are the minimal set of neuronal events and mechanisms observed to occur along with the mental states to which they are related."* (supports)

**Topic anchors**: why-anything-feels-like-anything, iit-as-a-theory-of-consciousness, could-machines-be-conscious

### NEURO-02: Lesion-Deficit Mapping

**Proposition**: Damage to specific brain regions produces reliable, predictable deficits in conscious experience and cognitive function.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Lesion` — *"a lesion in the central nervous system is called a central lesion, and a lesion in the peripheral nervous system is called a peripheral lesion"* (supports — placeholder; replace with a canonical neurology source quote at next ingestion pass)

**Topic anchors**: why-anything-feels-like-anything, iit-as-a-theory-of-consciousness

### NEURO-03: Anesthesia and Behavioral Consciousness

**Proposition**: General anesthesia reliably eliminates all behavioral indicators of conscious experience while preserving brainstem-mediated vital functions.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/General_anaesthesia` — *"General anaesthesia (UK) or general anesthesia (US) is medically induced loss of consciousness that renders a patient unarousable even by painful stimuli."* (supports)

**Topic anchors**: why-anything-feels-like-anything, iit-as-a-theory-of-consciousness, emergence-strong-vs-weak

---

## Biology and Epistemology

### BIO-01: Natural Selection

**Proposition**: Organisms with heritable traits that increase reproductive fitness in a given environment tend to increase in frequency across generations.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Natural_selection` — *"Natural selection is the differential survival and reproduction of individuals due to differences in the relative fitness endowed on them by their own particular complement of observable characteristics."* (supports)

**Topic anchors**: evolved-behavioral-adaptations, where-is-everybody, how-life-began

### EPIST-01: Open Science Collaboration Replication Results

**Proposition**: The Open Science Collaboration (2015) attempted to replicate 100 published psychology studies and found that only 36% of replications achieved statistically significant results, compared to 97% of original studies.

**Adjudication**: TRUE

**Citations**:
- Open Science Collaboration (2015), "Estimating the reproducibility of psychological science", Science 349 (6251) — DOI 10.1126/science.aac4716, ingestion target
- `https://en.wikipedia.org/wiki/Replication_crisis` — *"36% replicated successfully (p value below 0.05), with effect sizes averaging half the original magnitude."* (supports)

**Topic anchors**: sciences-self-correction-problem, how-science-decides-whats-true

---

## Computer Science and AI

### CS-01: LLM Training Objective

**Proposition**: Current large language models (GPT, Claude, Gemini, Llama families) are primarily trained via next-token prediction on large text corpora, with subsequent fine-tuning stages.

**Adjudication**: TRUE

**Citations**:
- `https://en.wikipedia.org/wiki/Large_language_model` — *"A large language model (LLM) is a neural network trained on a vast amount of text for natural language processing tasks, especially language generation."* (supports)
- Original technical reports (GPT, Claude, etc.) — ingestion targets where publicly available

**Topic anchors**: do-language-models-understand, can-syntax-produce-semantics, what-ai-can-do-now-and-where-its-going

---

## Adjudicated FALSE

### FALSE-01: Local Hidden Variables

**Proposition**: Local hidden variable theories can account for all predictions of quantum mechanics.

**Adjudication**: FALSE

**Citations**:
- See PHYS-01 (Bell violations) — same evidence, opposite polarity
- `https://en.wikipedia.org/wiki/Hidden-variable_theory` — *"In physics, a hidden-variable theory is a deterministic model which seeks to explain the probabilistic nature of quantum mechanics by introducing additional, possibly inaccessible, variables."* (supports the FALSE adjudication — the article's broader argument is that local versions are ruled out by Bell's theorem)

**Topic anchors**: capabilities-and-limits-of-quantum-computation

### FALSE-02: Known Polynomial-Time Algorithm for an NP-Complete Problem

**Proposition**: A polynomial-time algorithm for at least one NP-complete problem has been discovered and verified as of 2026.

**Adjudication**: FALSE

**Citations**:
- `https://en.wikipedia.org/wiki/P_versus_NP_problem` — *"problem itself remains open despite a million-dollar prize and a huge amount of dedicated research"* (supports the FALSE adjudication)

**Topic anchors**: the-most-important-open-problem-in-computer-science

### FALSE-03: FTL Information Transfer via Entanglement

**Proposition**: Quantum entanglement enables faster-than-light transmission of usable information.

**Adjudication**: FALSE

**Citations**:
- See PHYS-02 (no-communication theorem) — establishes the FALSE adjudication
- `https://en.wikipedia.org/wiki/Quantum_entanglement` — *"cannot be used for faster-than-light communication"* (supports the FALSE adjudication)

**Topic anchors**: capabilities-and-limits-of-quantum-computation
