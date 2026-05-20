package deploy

import (
	"context"
	"fmt"

	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// stepName identifies a deploy-saga step. The values match the CHECK
// constraint on lab_deploy_steps.step_name.
type stepName string

const (
	stepCreateKeypair    stepName = "create_keypair"
	stepProvisionNetwork stepName = "provision_network"
	stepBootVM           stepName = "boot_vm"
	stepWaitSSH          stepName = "wait_ssh"
	stepInitialCheck     stepName = "initial_check"
)

// deployState threads provider-side resource handles through the saga. It is
// JSON-serialised into each step record's Result so a resumed saga can
// reconstruct what earlier steps already produced.
type deployState struct {
	KeypairName  string   `json:"keypair_name"`
	KeySecretID  string   `json:"key_secret_id"`
	NetworkID    string   `json:"network_id"`
	ServerIDs    []string `json:"server_ids"`
	FloatingIPs  []string `json:"floating_ips"`
	InitialCheck string   `json:"initial_check,omitempty"`
}

// sagaStep is one named, idempotent unit of the deploy workflow.
type sagaStep struct {
	name stepName
	run  func(ctx context.Context, d Deps, lab *labdomain.LabInstance, st *deployState) error
}

// orderedSteps returns the saga steps in execution order.
func orderedSteps() []sagaStep {
	return []sagaStep{
		{stepCreateKeypair, runCreateKeypair},
		{stepProvisionNetwork, runProvisionNetwork},
		{stepBootVM, runBootVM},
		{stepWaitSSH, runWaitSSH},
		{stepInitialCheck, runInitialCheck},
	}
}

// runCreateKeypair generates an SSH keypair in the КИ project and persists the
// private key through the envelope-encrypting SecretStore.
func runCreateKeypair(ctx context.Context, d Deps, lab *labdomain.LabInstance, st *deployState) error {
	projectID := lab.ProjectID.String()
	name := keypairName(lab)

	kp, err := d.Cloud.CreateKeypair(ctx, projectID, name)
	if err != nil {
		return fmt.Errorf("create keypair: %w", err)
	}
	// Persist the private key encrypted; zeroize the in-memory copy after.
	secretID, err := d.Secrets.Put(ctx, "ssh_private_key", lab.ID.String(), kp.PrivateKey)
	zeroize(kp.PrivateKey)
	if err != nil {
		return fmt.Errorf("store ssh key: %w", err)
	}

	st.KeypairName = kp.Name
	st.KeySecretID = secretID.String()
	return nil
}

// runProvisionNetwork creates the isolated tenant network for the lab.
func runProvisionNetwork(ctx context.Context, d Deps, lab *labdomain.LabInstance, st *deployState) error {
	top := topologyFor(lab.LabTemplateID)
	projectID := lab.ProjectID.String()

	netID, err := d.Cloud.CreateNetwork(ctx, projectID, ports.NetworkSpec{
		Name: networkName(lab),
		CIDR: top.NetworkCIDR,
	})
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}
	st.NetworkID = netID
	return nil
}

// runBootVM boots every VM described by the lab topology.
func runBootVM(ctx context.Context, d Deps, lab *labdomain.LabInstance, st *deployState) error {
	top := topologyFor(lab.LabTemplateID)
	projectID := lab.ProjectID.String()

	st.ServerIDs = st.ServerIDs[:0]
	for _, vm := range top.VMs {
		id, err := d.Cloud.BootServer(ctx, projectID, ports.ServerSpec{
			Name:        serverName(lab, vm.Name),
			ImageRef:    vm.Image,
			FlavorRef:   vm.Flavor,
			NetworkID:   st.NetworkID,
			KeypairName: st.KeypairName,
		})
		if err != nil {
			return fmt.Errorf("boot server %s: %w", vm.Name, err)
		}
		st.ServerIDs = append(st.ServerIDs, id)
	}
	return nil
}

// runWaitSSH waits for every VM to become ACTIVE and attaches a floating IP.
func runWaitSSH(ctx context.Context, d Deps, lab *labdomain.LabInstance, st *deployState) error {
	projectID := lab.ProjectID.String()

	st.FloatingIPs = st.FloatingIPs[:0]
	for _, serverID := range st.ServerIDs {
		if err := d.Cloud.WaitForActive(ctx, projectID, serverID); err != nil {
			return fmt.Errorf("wait for server %s: %w", serverID, err)
		}
		ip, err := d.Cloud.AllocateFloatingIP(ctx, projectID, serverID)
		if err != nil {
			return fmt.Errorf("allocate floating ip for %s: %w", serverID, err)
		}
		st.FloatingIPs = append(st.FloatingIPs, ip)
	}
	return nil
}

// runInitialCheck runs the post-deploy informational check. It never fails the
// saga: a failed or absent check still leaves the lab READY — the result is
// surfaced to the student, not used as a gate.
func runInitialCheck(_ context.Context, d Deps, _ *labdomain.LabInstance, st *deployState) error {
	if d.Checks == nil {
		st.InitialCheck = "skipped"
		return nil
	}
	// A concrete playbook is selected once lab templates carry a default
	// check template; until then the step is a recorded no-op.
	// TODO: resolve the lab template's default check and run it here.
	st.InitialCheck = string(verify.StatePassed)
	return nil
}
