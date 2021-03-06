require 'spec_helper'

module Bosh::Director
  describe Jobs::RunErrand do
    subject(:job) { described_class.new('fake-dep-name', 'fake-errand-name') }

    before do
      App.stub_chain(:instance, :blobstores, :blobstore).
        with(no_args).
        and_return(blobstore)
    end
    let(:blobstore) { instance_double('Bosh::Blobstore::Client') }

    before { job.task_id = 'some-task' }

    describe 'Resque job class expectations' do
      let(:job_type) { :run_errand }
      it_behaves_like 'a Resque job'
    end

    describe '#perform' do
      context 'when deployment exists' do
        let!(:deployment_model) do
          Models::Deployment.make(
            name: 'fake-dep-name',
            manifest: "---\nmanifest: true",
          )
        end

        before { allow(Config).to receive(:event_log).with(no_args).and_return(event_log) }
        let(:event_log) { instance_double('Bosh::Director::EventLog::Log') }

        before do
          allow(DeploymentPlan::Planner).to receive(:parse).
            with({'manifest' => true}, event_log, {}).
            and_return(deployment)
        end
        let(:deployment) { instance_double('Bosh::Director::DeploymentPlan::Planner', name: 'deployment') }

        context 'when job representing an errand exists' do
          before { allow(deployment).to receive(:job).with('fake-errand-name').and_return(deployment_job) }
          let(:deployment_job) { instance_double('Bosh::Director::DeploymentPlan::Job', name: 'fake-errand-name') }

          context "when job can run as an errand (usually means lifecycle: errand)" do
            before { allow(deployment_job).to receive(:can_run_as_errand?).and_return(true) }

            context 'when job has at least 1 instance' do
              before { allow(deployment_job).to receive(:instances).with(no_args).and_return([instance]) }
              let(:instance) { instance_double('Bosh::Director::DeploymentPlan::Instance') }

              before { allow(Config).to receive(:result).with(no_args).and_return(result_file) }
              let(:result_file) { instance_double('Bosh::Director::TaskResultFile') }

              before do
                allow(Lock).to receive(:new).with('lock:deployment:deployment', timeout: 10).and_return(lock)
                allow(lock).to receive(:lock).and_yield
              end
              let(:lock) { instance_double('Bosh::Director::Lock') }

              before { allow(deployment_job).to receive(:resource_pool).with(no_args).and_return(resource_pool) }
              let(:resource_pool) { instance_double('Bosh::Director::DeploymentPlan::ResourcePool') }

              before do
                allow(Errand::DeploymentPreparer).to receive(:new).
                  with(deployment, deployment_job, event_log, subject).
                  and_return(deployment_preparer)
              end
              let(:deployment_preparer) do
                instance_double(
                  'Bosh::Director::Errand::DeploymentPreparer',
                  prepare_deployment: nil,
                  prepare_job: nil,
                )
              end

              before do
                allow(ResourcePoolUpdater).to receive(:new).
                  with(resource_pool).
                  and_return(rp_updater)
              end
              let(:rp_updater) { instance_double('Bosh::Director::ResourcePoolUpdater') }

              before do
                allow(DeploymentPlan::ResourcePools).to receive(:new).
                  with(event_log, [rp_updater]).
                  and_return(rp_manager)
              end
              let(:rp_manager) { instance_double('Bosh::Director::DeploymentPlan::ResourcePools', update: nil, refill: nil) }

              before do
                allow(Errand::JobManager).to receive(:new).
                  with(deployment, deployment_job, blobstore, event_log).
                  and_return(job_manager)
              end
              let(:job_manager) { instance_double('Bosh::Director::Errand::JobManager', update_instances: nil, delete_instances: nil) }

              before do
                allow(Errand::Runner).to receive(:new).
                  with(deployment_job, result_file, be_a(Api::InstanceManager), event_log).
                  and_return(runner)
              end
              let(:runner) { instance_double('Bosh::Director::Errand::Runner') }

              it 'runs an errand with deployment lock and returns short result description' do
                called_after_block_check = double(:called_in_block_check, call: nil)
                expect(subject).to receive(:with_deployment_lock) do |deployment, &blk|
                  result = blk.call
                  called_after_block_check.call
                  result
                end

                expect(deployment_preparer).to receive(:prepare_deployment).with(no_args).ordered
                expect(deployment_preparer).to receive(:prepare_job).with(no_args).ordered

                expect(rp_manager).to receive(:update).with(no_args).ordered

                expect(job_manager).to receive(:update_instances).with(no_args).ordered

                expect(runner).to receive(:run).
                  with(no_args).
                  ordered.
                  and_return('fake-result-short-description')

                expect(job_manager).to receive(:delete_instances).with(no_args).ordered
                expect(rp_manager).to receive(:refill).with(no_args).ordered

                expect(called_after_block_check).to receive(:call).ordered

                expect(subject.perform).to eq('fake-result-short-description')
              end

              context 'when the errand fails to run' do
                let(:task) { instance_double('Bosh::Director::Models::Task') }
                let(:task_manager) { instance_double('Bosh::Director::Api::TaskManager', find_task: task) }

                it 'cleans up the instances anyway' do
                  expect(runner).to receive(:run).with(no_args).and_raise(RuntimeError)

                  expect(job_manager).to receive(:delete_instances).with(no_args).ordered
                  expect(rp_manager).to receive(:refill).with(no_args).ordered

                  expect { subject.perform }.to raise_error(RuntimeError)
                end
              end

              context 'when the errand is canceled' do
                before { allow(Api::TaskManager).to receive(:new).and_return(task_manager) }
                let(:task_manager) { instance_double('Bosh::Director::Api::TaskManager', find_task: task) }
                let(:task) { instance_double('Bosh::Director::Models::Task') }

                before { allow(task).to receive(:state).and_return('cancelling') }

                it 'cancels the errand, raises TaskCancelled, and cleans up errand VMs' do
                  expect(job_manager).to receive(:update_instances).with(no_args).ordered

                  expect(runner).to receive(:run).with(no_args).ordered.and_yield

                  expect(runner).to receive(:cancel).with(no_args).ordered

                  expect(job_manager).to receive(:delete_instances).with(no_args).ordered
                  expect(rp_manager).to receive(:refill).with(no_args).ordered

                  expect { subject.perform }.to raise_error(TaskCancelled)
                end

                it 'does not allow cancellation within the cleanup' do
                  expect(job_manager).to receive(:update_instances).with(no_args).ordered

                  expect(runner).to receive(:run).with(no_args).ordered.and_yield

                  expect(runner).to receive(:cancel).with(no_args).ordered

                  expect(job_manager).to receive(:delete_instances) do
                    job.task_checkpoint
                  end.ordered
                  expect(rp_manager).to receive(:refill).with(no_args).ordered

                  expect { subject.perform }.to raise_error(TaskCancelled)
                end

                context 'when the agent throws an exception' do
                  it 'raises RpcRemoteException and cleans up errand VMs' do
                    expect(job_manager).to receive(:update_instances).with(no_args).ordered

                    expect(runner).to receive(:run).with(no_args).ordered.and_yield

                    expect(runner).to receive(:cancel).with(no_args).ordered.and_raise(RpcRemoteException)

                    expect(job_manager).to receive(:delete_instances).with(no_args).ordered
                    expect(rp_manager).to receive(:refill).with(no_args).ordered

                    expect { subject.perform }.to raise_error(RpcRemoteException)
                  end
                end
              end
            end

            context 'when job representing an errand has 0 instances' do
              before { allow(deployment_job).to receive(:instances).with(no_args).and_return([]) }

              it 'raises an error because errand cannot be run on a job without 0 instances' do
                expect {
                  subject.perform
                }.to raise_error(InstanceNotFound, %r{fake-errand-name/0.*doesn't exist})
              end
            end
          end

          context "when job cannot run as an errand (e.g. marked as 'lifecycle: service')" do
            before { allow(deployment_job).to receive(:can_run_as_errand?).and_return(false) }

            it 'raises an error because non-errand jobs cannot be used with run errand cmd' do
              expect {
                subject.perform
              }.to raise_error(RunErrandError, /Job `fake-errand-name' is not an errand/)
            end
          end
        end

        context 'when job representing an errand does not exist' do
          before { allow(deployment).to receive(:job).with('fake-errand-name').and_return(nil) }

          it 'raises an error because user asked to run an unknown errand' do
            expect {
              subject.perform
            }.to raise_error(JobNotFound, %r{fake-errand-name.*doesn't exist})
          end
        end
      end

      context 'when deployment does not exist' do
        it 'raises an error' do
          expect {
            subject.perform
          }.to raise_error(DeploymentNotFound, %r{fake-dep-name.*doesn't exist})
        end
      end
    end

    describe '#task_checkpoint' do
      subject { job.task_checkpoint }

      it_behaves_like 'raising an error when a task has timed out or been canceled'

      context 'when cancellation is ignored' do
        it 'does not raise an error' do
          job.send(:ignore_cancellation) do
            expect { job.task_checkpoint }.not_to raise_error
          end
        end
      end
    end
  end
end
